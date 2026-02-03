package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/config"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/git"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
	"github.com/spf13/cobra"
)

const (
	gitHost = "git.gethatch.eu"
)

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
	// Load config for app domain
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Determine app domain: config > env var > default
	appDomain := cfg.AppDomain
	if appDomain == "" {
		appDomain = os.Getenv("HATCH_APP_DOMAIN")
	}
	if appDomain == "" {
		appDomain = "gethatch.eu"
	}

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

	// 4. Determine app name
	name := appName
	if name == "" {
		cwd, err := deps.GetCwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w", err)
		}
		name = filepath.Base(cwd)
	}

	// 5. Build remote URL (token as password for Basic Auth)
	remoteURL := fmt.Sprintf("https://x:%s@%s/%s.git", token, gitHost, name)

	// 6. Setup/update hatch remote
	if deps.HasRemote("hatch") {
		if err := deps.SetRemoteURL("hatch", remoteURL); err != nil {
			return fmt.Errorf("updating hatch remote: %w", err)
		}
	} else {
		if err := deps.AddRemote("hatch", remoteURL); err != nil {
			return fmt.Errorf("adding hatch remote: %w", err)
		}
	}

	// 7. Get current branch and push
	branch, err := deps.CurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	// Push current branch as main to hatch remote
	pushRef := branch + ":main"

	sp := ui.NewSpinner("Deploying to Hatch...")
	sp.Start()
	output, pushErr := deps.Push("hatch", pushRef)
	sp.Stop()

	if pushErr != nil {
		return fmt.Errorf("push failed: %s", strings.TrimSpace(output))
	}

	// 8. Parse output for app URL and show success
	appURL := parseAppURL(output)
	fmt.Println()
	ui.Success("Deployed to Hatch!")

	// 9. Set custom domain if specified
	if domainName != "" {
		ui.Info(fmt.Sprintf("Configuring custom domain: %s", domainName))
		client := api.NewClient(token)
		domain, err := client.AddDomain(name, domainName)
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
		ui.Info(fmt.Sprintf("App URL: https://%s.%s", name, appDomain))
	}
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  hatch info     - View app details")
	fmt.Println("  hatch logs     - View app logs")
	fmt.Println("  hatch open     - Open app in browser")

	return nil
}

// parseAppURL extracts the app URL from git push output.
func parseAppURL(output string) string {
	// Try to find any HTTPS URL that looks like an app URL
	urlPattern := regexp.MustCompile(`https?://[a-zA-Z0-9.-]+(\.[a-zA-Z]{2,})?/[^\s]*`)
	match := urlPattern.FindString(output)
	return match
}

func getCwd() (string, error) {
	return filepath.Abs(".")
}
