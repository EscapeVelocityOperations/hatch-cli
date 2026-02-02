package deploy

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/git"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
	"github.com/spf13/cobra"
)

const (
	gitHost    = "git.gethatch.eu"
	deployPath = "/deploy"
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

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy your application to Hatch",
		Long:  "Deploy the current directory as a Hatch application. Initializes git if needed, commits changes, and pushes to the Hatch platform.",
		RunE:  runDeploy,
	}
	cmd.Flags().StringVarP(&appName, "name", "n", "", "custom app name (defaults to directory name)")
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

	// 4. Determine app name
	name := appName
	if name == "" {
		cwd, err := deps.GetCwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w", err)
		}
		name = filepath.Base(cwd)
	}

	// 5. Build remote URL
	remoteURL := fmt.Sprintf("https://%s@%s%s/%s.git", token, gitHost, deployPath, name)

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
	if appURL != "" {
		ui.Info(fmt.Sprintf("App URL: %s", appURL))
	} else {
		ui.Info(fmt.Sprintf("App URL: https://%s.gethatch.eu", name))
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
