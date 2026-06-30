// Package ci implements the `hatch ci` assist command: it detects the CI provider
// and project runtime and previews the deploy pipeline hatch would wire up. Per
// the assist boundary it explains and (in follow-ups) generates files, but never
// pushes — secret values are minted/stored by the user.
package ci

import (
	"fmt"
	"os"
	"os/exec"

	detect "github.com/EscapeVelocityOperations/hatch-cli/internal/ci"
	"github.com/spf13/cobra"
)

// Deps holds injectable dependencies for testing.
type Deps struct {
	GitRemote func() (string, error)
	Getwd     func() (string, error)
}

func defaultDeps() *Deps {
	return &Deps{
		GitRemote: func() (string, error) {
			out, err := exec.Command("git", "remote", "get-url", "origin").Output()
			return string(out), err
		},
		Getwd: os.Getwd,
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
		Short: "Set up a CI/CD deploy pipeline for your app",
		Long: "Detect your CI provider and project runtime and preview the deploy pipeline " +
			"hatch will wire up. This prints a plan only — it never writes files or pushes secrets.",
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
	f.BoolVar(&opts.printOnly, "print", false, "Print the generated config to stdout only (no file writes)")
	f.BoolVar(&opts.overwrite, "yes", false, "Allow overwriting existing CI config files")
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

	files := planFiles(provider)
	if len(files) == 0 {
		fmt.Fprintf(out, "\n  Provider %q not recognized — pass --provider github|gitlab to continue.\n", provider)
	} else {
		fmt.Fprintln(out, "  Will generate:")
		for _, file := range files {
			fmt.Fprintf(out, "    %s\n", file)
		}
	}
	fmt.Fprintf(out, "  CI secret: HATCH_TOKEN  (mint with: hatch auth keys create)\n")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Preview only — no files were written and nothing was pushed.")
	return nil
}

// planFiles returns the CI config file(s) the given provider would generate.
// (Generation itself lands in a follow-up; this is the assist preview.)
func planFiles(provider string) []string {
	switch provider {
	case "github":
		return []string{".github/workflows/hatch-deploy.yml"}
	case "gitlab":
		return []string{".gitlab-ci.yml"}
	default:
		return nil
	}
}
