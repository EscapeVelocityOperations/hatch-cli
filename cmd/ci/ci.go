// Package ci implements the `hatch ci` assist command: it detects the CI provider
// and project runtime and GENERATES the deploy workflow file. Per the assist
// boundary it writes the file into the repo (the user reviews + commits) but never
// commits or pushes, and never writes secret values — HATCH_TOKEN is minted/stored
// by the user.
package ci

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	detect "github.com/EscapeVelocityOperations/hatch-cli/internal/ci"
	"github.com/spf13/cobra"
)

// Deps holds injectable dependencies for testing.
type Deps struct {
	GitRemote func() (string, error)
	Getwd     func() (string, error)
	WriteFile func(path string, content []byte) error // creates parent dirs
	Stat      func(path string) bool                  // reports whether path exists
}

func defaultDeps() *Deps {
	return &Deps{
		GitRemote: func() (string, error) {
			out, err := exec.Command("git", "remote", "get-url", "origin").Output()
			return string(out), err
		},
		Getwd: os.Getwd,
		WriteFile: func(path string, content []byte) error {
			if dir := filepath.Dir(path); dir != "." && dir != "" {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return err
				}
			}
			return os.WriteFile(path, content, 0o644)
		},
		Stat: func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		},
	}
}

var deps = defaultDeps()

type ciOptions struct {
	provider     string
	app          string
	runtime      string
	deployTarget string
	startCommand string
	printOnly    bool
	overwrite    bool
}

// NewCmd returns the `hatch ci` command.
func NewCmd() *cobra.Command {
	var opts ciOptions
	cmd := &cobra.Command{
		Use:   "ci",
		Short: "Generate a CI/CD deploy pipeline for your app",
		Long: "Detect your CI provider and project runtime and generate the deploy workflow " +
			"(GitHub Actions or GitLab CI). Writes the file for you to review + commit; it never " +
			"commits, pushes, or writes secret values. Use --print to preview without writing.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCI(cmd, opts)
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.provider, "provider", "", "CI provider override (github, gitlab)")
	f.StringVar(&opts.app, "app", "", "Hatch app slug to deploy")
	f.StringVar(&opts.runtime, "runtime", "", "Runtime override (node, go, python, rust, php, bun, static)")
	f.StringVar(&opts.deployTarget, "deploy-target", "", "Build output directory to deploy")
	f.StringVar(&opts.startCommand, "start-command", "", "Start command for the app")
	f.BoolVar(&opts.printOnly, "print", false, "Print the generated workflow to stdout only (no file writes)")
	f.BoolVar(&opts.overwrite, "yes", false, "Allow overwriting an existing CI workflow file")
	return cmd
}

func runCI(cmd *cobra.Command, opts ciOptions) error {
	provider := opts.provider
	if provider == "" {
		if remote, err := deps.GitRemote(); err == nil {
			provider = detect.DetectProvider(remote)
		} else {
			provider = "unknown"
		}
	}

	runtime := opts.runtime
	if runtime == "" {
		if wd, err := deps.Getwd(); err == nil {
			runtime = detect.DetectRuntime(wd)
		} else {
			runtime = "static"
		}
	}

	app := opts.app
	if app == "" {
		app = "(set with --app)"
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "hatch ci — deploy pipeline assistant")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  Provider:  %s\n", provider)
	fmt.Fprintf(out, "  Runtime:   %s\n", runtime)
	fmt.Fprintf(out, "  App:       %s\n", app)
	fmt.Fprintf(out, "  CI secret: HATCH_TOKEN  (mint with: hatch auth keys create)\n")

	wf, err := detect.RenderWorkflow(provider, detect.WorkflowParams{
		Runtime:      runtime,
		DeployTarget: opts.deployTarget,
		StartCommand: opts.startCommand,
	})
	if err != nil {
		fmt.Fprintf(out, "\n  Provider %q not recognized — pass --provider github|gitlab.\n", provider)
		return err
	}

	if opts.printOnly {
		fmt.Fprintf(out, "\n--- %s ---\n%s", wf.Path, wf.Content)
		return nil
	}

	if deps.Stat(wf.Path) && !opts.overwrite {
		return fmt.Errorf("%s already exists — re-run with --yes to overwrite", wf.Path)
	}
	if err := deps.WriteFile(wf.Path, []byte(wf.Content)); err != nil {
		return fmt.Errorf("writing %s: %w", wf.Path, err)
	}

	fmt.Fprintf(out, "\nWrote %s\n", wf.Path)
	fmt.Fprintln(out, "Next: review + commit it, then add HATCH_TOKEN as a CI secret")
	fmt.Fprintln(out, "(mint one with: hatch auth keys create). Nothing was committed or pushed.")
	return nil
}
