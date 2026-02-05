package deploy

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/git"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
	"github.com/spf13/cobra"
)

const (
	gitHost = "git.gethatch.eu"
)

// HatchConfig represents the .hatch.toml config file.
type HatchConfig struct {
	Slug      string `toml:"slug"`
	Name      string `toml:"name"`
	CreatedAt string `toml:"created_at"`
}

// readHatchConfig reads .hatch.toml from the local git repo if it exists.
func readHatchConfig() (*HatchConfig, error) {
	cmd := exec.Command("git", "show", "hatch:.hatch.toml")
	output, err := cmd.Output()
	if err != nil {
		return nil, nil // Not an error, just doesn't exist
	}

	var config HatchConfig
	if err := toml.Unmarshal(output, &config); err != nil {
		return nil, fmt.Errorf("parsing .hatch.toml: %w", err)
	}

	if config.Slug == "" {
		return nil, fmt.Errorf("invalid .hatch.toml: missing slug")
	}

	return &config, nil
}

// APIClient is the interface for the Hatch API.
type APIClient interface {
	CreateApp(name string) (*api.App, error)
	RestartApp(slug string) error
}

// Deps holds injectable dependencies for testing.
type Deps struct {
	GetToken      func() (string, error)
	IsGitRepo     func() bool
	GitInit       func() error
	HasChanges    func() (bool, error)
	CommitAll     func(msg string) error
	HasRemote     func(name string) bool
	AddRemote     func(name, url string) error
	SetRemoteURL  func(name, url string) error
	GetRemoteURL  func(name string) (string, error)
	Push          func(remote, branch string) (string, error)
	CurrentBranch func() (string, error)
	GetCwd        func() (string, error)
	NewAPIClient  func(token string) APIClient
}

// apiClientWrapper wraps api.Client to implement APIClient interface.
type apiClientWrapper struct {
	*api.Client
}

func (w *apiClientWrapper) CreateApp(name string) (*api.App, error) {
	return w.Client.CreateApp(name)
}

func (w *apiClientWrapper) RestartApp(slug string) error {
	return w.Client.RestartApp(slug)
}

func defaultDeps() *Deps {
	return &Deps{
		GetToken:      auth.GetToken,
		IsGitRepo:     git.IsGitRepo,
		GitInit:       git.Init,
		HasChanges:    git.HasChanges,
		CommitAll:     git.CommitAll,
		HasRemote:     git.HasRemote,
		AddRemote:     git.AddRemote,
		SetRemoteURL:  git.SetRemoteURL,
		GetRemoteURL:  git.GetRemoteURL,
		Push:          git.Push,
		CurrentBranch: git.CurrentBranch,
		GetCwd:        getCwd,
		NewAPIClient: func(token string) APIClient {
			return &apiClientWrapper{api.NewClient(token)}
		},
	}
}

var deps = defaultDeps()

var appName string
var domainName string

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy your application to Hatch",
		Long:  "Deploy the current directory as a Hatch application. Initializes git if needed, commits changes, and pushes to the Hatch platform.",
		RunE:  runDeploy,
	}
	cmd.Flags().StringVarP(&appName, "name", "n", "", "custom app name (defaults to directory name)")
	cmd.Flags().StringVarP(&domainName, "domain", "d", "", "custom domain (e.g. example.com)")
	return cmd
}

func runDeploy(cmd *cobra.Command, args []string) error {
	// 1. Check auth
	token, err := deps.GetToken()
	if err != nil {
		return fmt.Errorf("checking auth: %w", err)
	}
	if token == "" {
		return fmt.Errorf("not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token")
	}

	// 2. Check/init git repo
	if !deps.IsGitRepo() {
		ui.Info("Initializing git repository...")
		if err := deps.GitInit(); err != nil {
			return fmt.Errorf("initializing git: %w", err)
		}
	}

	// 3. Auto-commit uncommitted changes
	hasChanges, err := deps.HasChanges()
	if err != nil {
		return fmt.Errorf("checking git status: %w", err)
	}
	if hasChanges {
		ui.Info("Committing changes...")
		if err := deps.CommitAll("Deploy via hatch"); err != nil {
			return fmt.Errorf("committing changes: %w", err)
		}
	}

	// 4. Check for .hatch.toml config to identify existing app
	ui.Info("Checking for .hatch.toml...")
	var slug string
	var isNewApp bool
	hatchConfig, err := readHatchConfig()
	if err != nil {
		return fmt.Errorf("reading .hatch.toml: %w", err)
	}

	client := deps.NewAPIClient(token)

	if hatchConfig != nil {
		// Found existing app config - use it
		ui.Info(fmt.Sprintf("Found existing app: %s (%s)", hatchConfig.Slug, hatchConfig.Name))
		slug = hatchConfig.Slug
		isNewApp = false
		if appName != "" && appName != hatchConfig.Name {
			ui.Warn("App name in .hatch.toml differs from --name flag, using .hatch.toml")
		}
	} else {
		// No config found - create new app
		// 4a. Determine app name
		name := appName
		if name == "" {
			cwd, err := deps.GetCwd()
			if err != nil {
				return fmt.Errorf("getting current directory: %w", err)
			}
			name = filepath.Base(cwd)
		}

		// 5. Create app via API to get the slug
		ui.Info("Creating new app...")
		app, err := client.CreateApp(name)
		if err != nil {
			return fmt.Errorf("creating app: %w", err)
		}
		slug = app.Slug
		isNewApp = true
	}

	// 6. Build remote URL with slug (token as username for Basic Auth)
	remoteURL := fmt.Sprintf("https://%s:x@%s/deploy/%s.git", token, gitHost, slug)

	// 7. Setup/update hatch remote
	if deps.HasRemote("hatch") {
		if err := deps.SetRemoteURL("hatch", remoteURL); err != nil {
			return fmt.Errorf("updating hatch remote: %w", err)
		}
	} else {
		if err := deps.AddRemote("hatch", remoteURL); err != nil {
			return fmt.Errorf("adding hatch remote: %w", err)
		}
	}

	// 8. Get current branch and push
	branch, err := deps.CurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	// Push current branch as main to hatch remote
	pushRef := branch + ":main"

	sp := ui.NewSpinner("Pushing to Hatch...")
	sp.Start()
	output, pushErr := deps.Push("hatch", pushRef)
	sp.Stop()

	if pushErr != nil {
		return fmt.Errorf("push failed: %s", strings.TrimSpace(output))
	}

	// 9. Trigger build/restart on the control plane
	ui.Info("Triggering build...")
	if !isNewApp {
		// Existing app: trigger restart to rebuild
		if err := client.RestartApp(slug); err != nil {
			return fmt.Errorf("triggering rebuild: %w", err)
		}
	}
	// For new apps, the build is triggered automatically by the git handler

	// 10. Show success message
	appURL := parseAppURL(output)
	fmt.Println()
	if !isNewApp {
		ui.Success("Updated existing app!")
	} else {
		ui.Success("Deployed to Hatch!")
	}

	// 11. Set custom domain if specified
	if domainName != "" {
		ui.Info(fmt.Sprintf("Configuring custom domain: %s", domainName))
		domainClient := api.NewClient(token)
		domain, err := domainClient.AddDomain(slug, domainName)
		if err != nil {
			ui.Warn(fmt.Sprintf("Domain configuration failed: %v", err))
			ui.Info("You can configure it later with: hatch domain add " + domainName)
		} else {
			ui.Success(fmt.Sprintf("Domain %s configured", domainName))
			if domain.CNAME != "" {
				ui.Info(fmt.Sprintf("CNAME target: %s", domain.CNAME))
			}
		}
	}

	if appURL != "" {
		ui.Info(fmt.Sprintf("App URL: %s", appURL))
	} else {
		ui.Info(fmt.Sprintf("App URL: https://%s.hosted.gethatch.eu", slug))
	}
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  hatch info     - View app details")
	fmt.Println("  hatch logs     - View app logs")
	fmt.Println("  hatch open     - Open app in browser")

	return nil
}

// parseAppURL extracts the app URL from git push output.
var urlPattern = regexp.MustCompile(`https?://[^\s]+\.gethatch\.eu[^\s]*`)

func parseAppURL(output string) string {
	match := urlPattern.FindString(output)
	return match
}

func getCwd() (string, error) {
	return filepath.Abs(".")
}
