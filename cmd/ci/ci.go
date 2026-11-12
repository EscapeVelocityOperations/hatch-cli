// Package ci implements the `hatch ci` assist command: it detects the CI provider
// and project runtime and GENERATES the deploy workflow file. Per the assist
// boundary it writes the file into the repo (the user reviews + commits) but never
// commits or pushes, and never writes secret values — HATCH_TOKEN is minted/stored
// by the user.
package ci

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	detect "github.com/EscapeVelocityOperations/hatch-cli/internal/ci"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/version"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Deps holds injectable dependencies for testing.
type Deps struct {
	GitRemote func() (string, error)
	Getwd     func() (string, error)
	WriteFile func(path string, content []byte) error // creates parent dirs
	Stat      func(path string) bool                  // reports whether path exists
	LookPath  func(bin string) bool                   // reports whether a binary is installed
	// Run executes a command, feeding stdin to its standard input. Secrets are
	// only ever passed this way, never in args.
	Run func(stdin io.Reader, bin string, args ...string) error
	// ReadToken reads the HATCH_TOKEN value for --set-secret from this
	// process's standard input: a masked prompt (written to prompt) on a
	// terminal, otherwise the piped content.
	ReadToken func(prompt io.Writer) (string, error)
	// FetchRelease resolves the sha256 pins of a published CLI release.
	FetchRelease func(version string) (detect.CLIRelease, error)
	// CLIVersion is the version of this binary (default --cli-version).
	CLIVersion func() string
	// GoMainPackages lists the main packages of the Go module in dir.
	GoMainPackages func(dir string) ([]string, error)
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
		LookPath: func(bin string) bool {
			_, err := exec.LookPath(bin)
			return err == nil
		},
		Run: func(stdin io.Reader, bin string, args ...string) error {
			c := exec.Command(bin, args...)
			c.Stdin = stdin
			return c.Run()
		},
		ReadToken:      readTokenFromStdin,
		FetchRelease:   detect.FetchCLIRelease,
		CLIVersion:     version.Version,
		GoMainPackages: detect.FindGoMainPackages,
	}
}

var deps = defaultDeps()

type ciOptions struct {
	provider     string
	app          string
	runtime      string
	deployTarget string
	startCommand string
	goPackage    string
	cliVersion   string
	printOnly    bool
	overwrite    bool
	setSecret    bool
}

// NewCmd returns the `hatch ci` command.
func NewCmd() *cobra.Command {
	var opts ciOptions
	cmd := &cobra.Command{
		Use:   "ci",
		Short: "Generate a CI/CD deploy pipeline for your app",
		Long: "Detect your CI provider and project runtime and generate the deploy workflow " +
			"(GitHub Actions or GitLab CI). Writes the file for you to review + commit; it never " +
			"commits, pushes, or writes secret values. Use --print to preview without writing.\n\n" +
			"The workflow deploys to --app (default: the slug in .hatch.toml) and needs --deploy-target " +
			"and, except for static sites, --start-command. It installs a pinned Hatch CLI release " +
			"(--cli-version, default: this CLI's version) whose sha256 is checked on the runner, and " +
			"sets HATCH_ACCEPT_TERMS=yes: committing it records your acceptance of https://gethatch.eu/terms " +
			"for that pipeline.\n\n" +
			"For the go runtime the workflow builds one main package into --deploy-target as a static " +
			"linux/amd64 binary, named by the start command (--start-command ./server builds " +
			"<deploy-target>/server). The package is --go-package, or the only main package of the " +
			"checkout, or the one whose directory is named like the binary; otherwise pass --go-package.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCI(cmd, opts)
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.provider, "provider", "", "CI provider override (github, gitlab)")
	f.StringVar(&opts.app, "app", "", "Hatch egg slug to deploy to (default: slug in .hatch.toml)")
	f.StringVar(&opts.runtime, "runtime", "", "Runtime override (node, go, python, rust, php, bun, static)")
	f.StringVar(&opts.deployTarget, "deploy-target", "", "Build output directory to deploy (required)")
	f.StringVar(&opts.startCommand, "start-command", "", "Start command for the app (required unless runtime is static)")
	f.StringVar(&opts.goPackage, "go-package", "", "Go main package to build into --deploy-target, e.g. ./cmd/server (go runtime; default: the only main package, or the one named like the start command's binary)")
	f.StringVar(&opts.cliVersion, "cli-version", "", "Hatch CLI release the workflow installs (default: this CLI's version)")
	f.BoolVar(&opts.printOnly, "print", false, "Print the generated workflow to stdout only (no file writes)")
	f.BoolVar(&opts.overwrite, "yes", false, "Allow overwriting an existing CI workflow file")
	f.BoolVar(&opts.setSecret, "set-secret", false, "Run the provider CLI (gh/glab) to set the HATCH_TOKEN secret; a hatch_ deploy token is read from stdin (masked prompt on a terminal, e.g. `hatch auth keys create | hatch ci … --set-secret`) and passed to gh/glab on stdin, never in arguments (default: print the command only)")
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

	wd, wdErr := deps.Getwd()

	runtime := opts.runtime
	if runtime == "" {
		if wdErr == nil {
			runtime = detect.DetectRuntime(wd)
		} else {
			runtime = "static"
		}
	}

	app := opts.app
	if app == "" && wdErr == nil {
		app = detect.AppSlugFromHatchToml(wd)
	}
	appLabel := app
	if appLabel == "" {
		appLabel = "(none — set with --app)"
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "hatch ci — deploy pipeline assistant")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  Provider:  %s\n", provider)
	fmt.Fprintf(out, "  Runtime:   %s\n", runtime)
	fmt.Fprintf(out, "  App:       %s\n", appLabel)
	fmt.Fprintf(out, "  CI secret: HATCH_TOKEN  (mint with: hatch auth keys create)\n")

	if provider != "github" && provider != "gitlab" {
		fmt.Fprintf(out, "\n  Provider %q not recognized — pass --provider github|gitlab.\n", provider)
		return fmt.Errorf("unsupported CI provider %q (use github or gitlab)", provider)
	}

	params := detect.WorkflowParams{
		App:          app,
		Runtime:      runtime,
		DeployTarget: opts.deployTarget,
		StartCommand: opts.startCommand,
		GoPackage:    opts.goPackage,
	}
	// Refuse an unusable or unsafe workflow before any network call or write.
	if err := params.ValidateInputs(); err != nil {
		return err
	}
	if runtime == "go" {
		pkg, err := resolveGoPackage(wd, wdErr, params)
		if err != nil {
			return err
		}
		params.GoPackage = pkg
		output, _ := detect.GoBuildOutput(params) // the start command is validated
		fmt.Fprintf(out, "  Go build:  %s -> %s (static linux/amd64)\n", pkg, output)
	}

	cliVersion := opts.cliVersion
	if cliVersion == "" {
		cliVersion = deps.CLIVersion()
	}
	if err := detect.CheckCIVersion(cliVersion); err != nil {
		return err
	}
	rel, err := deps.FetchRelease(cliVersion)
	if err != nil {
		return err
	}
	params.CLI = rel
	fmt.Fprintf(out, "  CLI:       %s (sha256-pinned)\n", rel.Version)

	wf, err := detect.RenderWorkflow(provider, params)
	if err != nil {
		return err
	}

	if opts.printOnly {
		fmt.Fprintf(out, "\n--- %s ---\n%s", wf.Path, wf.Content)
	} else {
		if deps.Stat(wf.Path) && !opts.overwrite {
			return fmt.Errorf("%s already exists — re-run with --yes to overwrite", wf.Path)
		}
		if err := deps.WriteFile(wf.Path, []byte(wf.Content)); err != nil {
			return fmt.Errorf("writing %s: %w", wf.Path, err)
		}
		fmt.Fprintf(out, "\nWrote %s (review + commit it; nothing was committed or pushed).\n", wf.Path)
		fmt.Fprintln(out, "Committing it accepts the Hatch Terms (https://gethatch.eu/terms) for that pipeline (HATCH_ACCEPT_TERMS).")
	}

	return wireSecret(cmd, opts, provider)
}

// resolveGoPackage returns the main package the go workflow builds. `go build
// ./...` compiles everything but writes no executable, so the workflow names
// the one package to build: --go-package (checked against the checkout's main
// packages, since building a library would deploy a non-executable), else the
// unambiguous main package of the checkout (h-8tvak.4).
func resolveGoPackage(wd string, wdErr error, params detect.WorkflowParams) (string, error) {
	if wdErr != nil {
		if params.GoPackage != "" {
			return params.GoPackage, nil // nothing to check it against
		}
		return "", fmt.Errorf("cannot find the Go main package to build without a working directory (%v): pass --go-package (for example ./cmd/server)", wdErr)
	}
	candidates, err := deps.GoMainPackages(wd)
	if err != nil {
		return "", fmt.Errorf("looking for the Go main package to build in %s: %w", wd, err)
	}
	if params.GoPackage != "" {
		if err := detect.VerifyGoMainPackage(candidates, params.GoPackage); err != nil {
			return "", err
		}
		return params.GoPackage, nil
	}
	return detect.ChooseGoMainPackage(candidates, params.StartCommand)
}

// wireSecret prints (or, under --set-secret, runs) the provider command that sets
// the HATCH_TOKEN CI secret. Default is print-only — it never silently pushes a
// secret. The token value is never printed nor placed in any argv: --set-secret
// reads it from this process's stdin (masked prompt on a terminal) and hands it
// to gh/glab on their stdin, after confirming the provider CLI is installed; any
// failure falls back to manual instructions.
func wireSecret(cmd *cobra.Command, opts ciOptions, provider string) error {
	out := cmd.OutOrStdout()
	ownerRepo := ""
	if remote, err := deps.GitRemote(); err == nil {
		ownerRepo = detect.OwnerRepoFromRemote(remote)
	}
	bin, args, ok := detect.SecretCommand(provider, ownerRepo)
	if !ok {
		return nil
	}
	manual := detect.SecretCommandString(provider, ownerRepo)

	// The provider CLI reads the token from a non-terminal stdin, and `hatch auth
	// keys create` writes the token alone on stdout when piped, so the pipe is the
	// one instruction that works for both providers without the token in argv.
	piped := "hatch auth keys create | " + manual

	if !opts.setSecret {
		fmt.Fprintln(out, "\nWire the HATCH_TOKEN secret in your CI — pipe a freshly minted token (shown once) into the provider CLI, which reads it from stdin:")
		fmt.Fprintf(out, "  %s\n", piped)
		fmt.Fprintln(out, "  (or re-run this command with --set-secret and the token piped into it: hatch auth keys create | hatch ci … --set-secret)")
		return nil
	}

	if !deps.LookPath(bin) {
		fmt.Fprintf(out, "\n%s is not installed — set the secret manually in your repo settings, or install it and run:\n  %s\n", bin, piped)
		return nil
	}
	token, err := deps.ReadToken(cmd.ErrOrStderr())
	if err != nil {
		return fmt.Errorf("reading the HATCH_TOKEN value from stdin: %w", err)
	}
	if token == "" {
		return errors.New("--set-secret needs the token value on stdin (mint with: hatch auth keys create)")
	}
	if err := deps.Run(strings.NewReader(token), bin, args...); err != nil {
		fmt.Fprintf(out, "\n%s failed (is it authenticated? try `%s auth login`). The token read from stdin was not stored; mint a new one and pipe it:\n  %s\n", bin, bin, piped)
		return nil
	}
	fmt.Fprintf(out, "\nSet the HATCH_TOKEN secret via %s.\n", bin)
	return nil
}

// readTokenFromStdin reads the token from os.Stdin: a masked prompt on a
// terminal (nothing echoed), otherwise the piped stream. Either way the value
// must be a hatch_ deploy token, extracted strictly: a piped stream may carry
// messages around the token (the human output of `hatch auth keys create`) and
// those are ignored, but a stream without exactly one token is refused rather
// than stored as the CI secret. Empty input returns "" with no error so
// wireSecret reports the missing value. Errors never echo the input.
func readTokenFromStdin(prompt io.Writer) (string, error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		fmt.Fprint(prompt, "HATCH_TOKEN (input hidden): ")
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(prompt)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(string(b)) == "" {
			return "", nil
		}
		return detect.ExtractToken(string(b))
	}
	return detect.ReadTokenFrom(os.Stdin)
}
