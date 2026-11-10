package ci

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	detect "github.com/EscapeVelocityOperations/hatch-cli/internal/ci"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ci/workflowtest"
)

// fakeRelease stands in for the checksums.txt download.
func fakeRelease(version string) (detect.CLIRelease, error) {
	sum := workflowtest.FakeHatchSHA256()
	return detect.CLIRelease{Version: version, SHA256AMD64: sum, SHA256ARM64: sum}, nil
}

// setMockDeps installs d, defaulting the release lookup and CLI version so
// tests never hit the network.
func setMockDeps(d *Deps) func() {
	if d.FetchRelease == nil {
		d.FetchRelease = fakeRelease
	}
	if d.CLIVersion == nil {
		d.CLIVersion = func() string { return "v0.21.0" }
	}
	old := deps
	deps = d
	return func() { deps = old }
}

// baseArgs are the deploy parameters every generation now requires.
var baseArgs = []string{"--app", "fixture-stage", "--deploy-target", "dist", "--start-command", "node server.js"}

func args(extra ...string) []string { return append(append([]string{}, baseArgs...), extra...) }

func execCI(t *testing.T, argv []string) (string, error) {
	t.Helper()
	cmd := NewCmd()
	cmd.SetArgs(argv)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := cmd.Execute()
	return buf.String(), err
}

// capture is a WriteFile/Stat pair that records the write and reports files absent.
type capture struct {
	path    string
	content []byte
	called  bool
}

func (c *capture) write(path string, content []byte) error {
	c.path, c.content, c.called = path, content, true
	return nil
}

// h-tymh/h-e2h5: `hatch ci` detects provider (origin remote) + runtime (project
// files) and GENERATES the workflow file for that combo (github + node here).
func TestCI_GeneratesWorkflow(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	var cap capture
	restore := setMockDeps(&Deps{
		GitRemote: func() (string, error) { return "git@github.com:acme/repo.git", nil },
		Getwd:     func() (string, error) { return dir, nil },
		Stat:      func(string) bool { return false },
		WriteFile: cap.write,
	})
	defer restore()

	cmd := NewCmd()
	cmd.SetArgs(args())
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if cap.path != ".github/workflows/hatch-deploy.yml" {
		t.Errorf("wrote %q, want .github/workflows/hatch-deploy.yml", cap.path)
	}
	if !strings.Contains(string(cap.content), "actions/setup-node") {
		t.Errorf("generated github/node workflow missing the node setup:\n%s", cap.content)
	}
	out := buf.String()
	for _, want := range []string{"github", "node", "HATCH_TOKEN", "Wrote .github/workflows/hatch-deploy.yml"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\noutput:\n%s", want, out)
		}
	}
}

// --provider / --runtime overrides bypass detection (non-repo dir still works).
func TestCI_ProviderAndRuntimeOverride(t *testing.T) {
	var cap capture
	restore := setMockDeps(&Deps{
		GitRemote: func() (string, error) { return "", os.ErrNotExist },
		Getwd:     func() (string, error) { return "", os.ErrNotExist },
		Stat:      func(string) bool { return false },
		WriteFile: cap.write,
	})
	defer restore()

	cmd := NewCmd()
	cmd.SetArgs(args("--provider", "gitlab", "--runtime", "go"))
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if cap.path != ".gitlab-ci.yml" {
		t.Errorf("wrote %q, want .gitlab-ci.yml", cap.path)
	}
	if !strings.Contains(string(cap.content), "go build") {
		t.Errorf("generated gitlab/go workflow missing the go build:\n%s", cap.content)
	}
}

// --print previews the workflow to stdout and writes NOTHING.
func TestCI_Print_WritesNothing(t *testing.T) {
	var cap capture
	restore := setMockDeps(&Deps{
		GitRemote: func() (string, error) { return "", os.ErrNotExist },
		Getwd:     func() (string, error) { return "", os.ErrNotExist },
		Stat:      func(string) bool { return false },
		WriteFile: cap.write,
	})
	defer restore()

	cmd := NewCmd()
	cmd.SetArgs(args("--provider", "github", "--runtime", "node", "--print"))
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if cap.called {
		t.Errorf("--print must NOT write a file; wrote %q", cap.path)
	}
	if !strings.Contains(buf.String(), "hatch\" deploy") {
		t.Errorf("--print must show the workflow content; got:\n%s", buf.String())
	}
}

// An existing workflow file is not overwritten without --yes.
func TestCI_OverwriteGuard(t *testing.T) {
	base := &Deps{
		GitRemote: func() (string, error) { return "", os.ErrNotExist },
		Getwd:     func() (string, error) { return "", os.ErrNotExist },
		Stat:      func(string) bool { return true }, // file already exists
	}

	// Without --yes: refuse (no write).
	var cap capture
	d1 := *base
	d1.WriteFile = cap.write
	restore := setMockDeps(&d1)
	cmd := NewCmd()
	cmd.SetArgs(args("--provider", "github", "--runtime", "node"))
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := cmd.Execute()
	restore()
	if err == nil {
		t.Fatal("expected an error when the workflow exists and --yes is absent")
	}
	if cap.called {
		t.Error("must not overwrite without --yes")
	}

	// With --yes: overwrite.
	var cap2 capture
	d2 := *base
	d2.WriteFile = cap2.write
	restore2 := setMockDeps(&d2)
	defer restore2()
	cmd2 := NewCmd()
	cmd2.SetArgs(args("--provider", "github", "--runtime", "node", "--yes"))
	buf2 := new(bytes.Buffer)
	cmd2.SetOut(buf2)
	cmd2.SetErr(buf2)
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("--yes should overwrite: %v", err)
	}
	if !cap2.called {
		t.Error("--yes must write the file")
	}
}

// h-mkyo: by default `hatch ci` PRINTS the secret-wiring command (never runs it).
func TestCI_SecretGuidance_DefaultPrintsCommand(t *testing.T) {
	restore := setMockDeps(&Deps{
		GitRemote: func() (string, error) { return "git@github.com:acme/repo.git", nil },
		Getwd:     func() (string, error) { return "", os.ErrNotExist },
		Stat:      func(string) bool { return false },
		WriteFile: func(string, []byte) error { return nil },
	})
	defer restore()
	cmd := NewCmd()
	cmd.SetArgs([]string{"--provider", "github", "--runtime", "static", "--app", "fixture-stage", "--deploy-target", "dist"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "gh secret set HATCH_TOKEN") {
		t.Errorf("default must print the secret-wiring command; got:\n%s", buf.String())
	}
}

// recorder captures a provider CLI invocation made through Deps.Run.
type recorder struct {
	ran   bool
	bin   string
	args  []string
	stdin string
}

func (r *recorder) run(stdin io.Reader, bin string, args ...string) error {
	r.ran, r.bin, r.args = true, bin, args
	if stdin != nil {
		b, _ := io.ReadAll(stdin)
		r.stdin = string(b)
	}
	return nil
}

// --set-secret reads the token via ReadToken and runs the provider CLI with it on stdin.
func TestCI_SetSecret_RunsViaRunner(t *testing.T) {
	rec := &recorder{}
	restore := setMockDeps(&Deps{
		GitRemote: func() (string, error) { return "git@github.com:acme/repo.git", nil },
		Getwd:     func() (string, error) { return "", os.ErrNotExist },
		Stat:      func(string) bool { return false },
		WriteFile: func(string, []byte) error { return nil },
		LookPath:  func(string) bool { return true },
		Run:       rec.run,
		ReadToken: func(io.Writer) (string, error) { return "hatch_secret", nil },
	})
	defer restore()
	out, err := execCI(t, args("--provider", "github", "--runtime", "node", "--set-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if rec.bin != "gh" {
		t.Errorf("ran %q, want gh", rec.bin)
	}
	if got := strings.Join(rec.args, " "); got != "secret set HATCH_TOKEN -R acme/repo" {
		t.Errorf("gh args = %q", got)
	}
	if rec.stdin != "hatch_secret" {
		t.Errorf("token must reach gh on stdin; got %q", rec.stdin)
	}
	if !strings.Contains(out, "Set the HATCH_TOKEN secret") {
		t.Errorf("missing success message:\n%s", out)
	}
}

// h-8tvak.1 (Vigie): on both providers, in the default print path and under
// --set-secret (success and provider failure), a synthetic token never appears
// in stdout/stderr, returned errors or the provider argv — it only reaches the
// provider CLI on stdin.
func TestCI_TokenNeverDisclosed(t *testing.T) {
	const sentinel = "SENTINEL-not-a-real-token-8tvak"
	for _, provider := range []string{"github", "gitlab"} {
		for _, mode := range []string{"print", "set-secret", "set-secret-provider-fails"} {
			t.Run(provider+"/"+mode, func(t *testing.T) {
				rec := &recorder{}
				run := rec.run
				if mode == "set-secret-provider-fails" {
					run = func(stdin io.Reader, bin string, args ...string) error {
						_ = rec.run(stdin, bin, args...)
						return errors.New("exit status 1")
					}
				}
				readCalled := false
				restore := setMockDeps(&Deps{
					GitRemote: func() (string, error) { return "git@example.com:acme/repo.git", nil },
					Getwd:     func() (string, error) { return "", os.ErrNotExist },
					Stat:      func(string) bool { return false },
					WriteFile: func(string, []byte) error { return nil },
					LookPath:  func(string) bool { return true },
					Run:       run,
					ReadToken: func(io.Writer) (string, error) { readCalled = true; return sentinel, nil },
				})
				defer restore()
				argv := args("--provider", provider, "--runtime", "node")
				if mode != "print" {
					argv = append(argv, "--set-secret")
				}
				out, err := execCI(t, argv)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if strings.Contains(out, sentinel) {
					t.Errorf("token disclosed in stdout/stderr:\n%s", out)
				}
				if mode == "print" {
					if rec.ran || readCalled {
						t.Error("default path must neither read the token nor run the provider CLI")
					}
					if !strings.Contains(out, "HATCH_TOKEN") {
						t.Errorf("default path must print the wiring command:\n%s", out)
					}
					return
				}
				if !rec.ran {
					t.Fatal("--set-secret must run the provider CLI")
				}
				if strings.Contains(strings.Join(rec.args, " "), sentinel) {
					t.Errorf("token disclosed in provider argv: %v", rec.args)
				}
				if rec.stdin != sentinel {
					t.Errorf("token must reach the provider CLI on stdin; got %q", rec.stdin)
				}
			})
		}
	}
}

// --set-secret with nothing on stdin is refused before running the provider CLI.
func TestCI_SetSecret_EmptyToken(t *testing.T) {
	rec := &recorder{}
	restore := setMockDeps(&Deps{
		GitRemote: func() (string, error) { return "git@github.com:acme/repo.git", nil },
		Getwd:     func() (string, error) { return "", os.ErrNotExist },
		Stat:      func(string) bool { return false },
		WriteFile: func(string, []byte) error { return nil },
		LookPath:  func(string) bool { return true },
		Run:       rec.run,
		ReadToken: func(io.Writer) (string, error) { return "", nil },
	})
	defer restore()
	_, err := execCI(t, args("--provider", "github", "--runtime", "node", "--set-secret"))
	if err == nil || !strings.Contains(err.Error(), "stdin") {
		t.Fatalf("want stdin error, got %v", err)
	}
	if rec.ran {
		t.Error("must not run the provider CLI without a token")
	}
}

// --token no longer exists: the value would sit in this process's argv.
func TestCI_TokenFlagRemoved(t *testing.T) {
	restore := setMockDeps(&Deps{
		GitRemote: func() (string, error) { return "git@github.com:acme/repo.git", nil },
		Getwd:     func() (string, error) { return "", os.ErrNotExist },
		Stat:      func(string) bool { return false },
		WriteFile: func(string, []byte) error { return nil },
	})
	defer restore()
	if _, err := execCI(t, args("--provider", "github", "--runtime", "node", "--token", "x")); err == nil {
		t.Fatal("--token must be rejected")
	}
}

// --set-secret with the provider CLI not installed falls back to manual instructions.
func TestCI_SetSecret_MissingCLI_Fallback(t *testing.T) {
	rec := &recorder{}
	readCalled := false
	restore := setMockDeps(&Deps{
		GitRemote: func() (string, error) { return "git@github.com:acme/repo.git", nil },
		Getwd:     func() (string, error) { return "", os.ErrNotExist },
		Stat:      func(string) bool { return false },
		WriteFile: func(string, []byte) error { return nil },
		LookPath:  func(string) bool { return false },
		Run:       rec.run,
		ReadToken: func(io.Writer) (string, error) { readCalled = true; return "t", nil },
	})
	defer restore()
	out, err := execCI(t, args("--provider", "github", "--runtime", "node", "--set-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if rec.ran || readCalled {
		t.Error("must neither prompt for the token nor run when the provider CLI is not installed")
	}
	if !strings.Contains(out, "not installed") {
		t.Errorf("missing not-installed fallback:\n%s", out)
	}
}

func noRepoDeps(cap *capture) *Deps {
	return &Deps{
		GitRemote: func() (string, error) { return "", os.ErrNotExist },
		Getwd:     func() (string, error) { return "", os.ErrNotExist },
		Stat:      func(string) bool { return false },
		WriteFile: cap.write,
	}
}

// h-cmzf.2: --app is carried into the generated workflow and wins over the
// slug in .hatch.toml; without --app the .hatch.toml slug is used.
func TestCI_AppTargetsTheWorkflow(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".hatch.toml"), []byte("[app]\nslug = \"fixture-prod\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gen := func(argv []string) string {
		var cap capture
		restore := setMockDeps(&Deps{
			GitRemote: func() (string, error) { return "https://github.com/acme/repo", nil },
			Getwd:     func() (string, error) { return dir, nil },
			Stat:      func(string) bool { return false },
			WriteFile: cap.write,
		})
		defer restore()
		if out, err := execCI(t, argv); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		d, err := workflowtest.Extract("github", string(cap.content))
		if err != nil {
			t.Fatal(err)
		}
		return d.Env["HATCH_CI_APP"]
	}
	common := []string{"--runtime", "node", "--deploy-target", "dist", "--start-command", "node server.js"}
	if got := gen(append([]string{"--app", "fixture-stage"}, common...)); got != "fixture-stage" {
		t.Errorf("--app fixture-stage: workflow targets %q", got)
	}
	if got := gen(common); got != "fixture-prod" {
		t.Errorf("no --app: workflow targets %q, want the .hatch.toml slug", got)
	}
}

// h-cmzf.2/.3: generation is refused — and nothing is written — when the
// pipeline could not deploy.
func TestCI_RefusesUnusableWorkflow(t *testing.T) {
	cases := map[string][]string{
		"no app anywhere":         {"--provider", "github", "--runtime", "node", "--deploy-target", "dist", "--start-command", "x"},
		"no deploy target":        {"--provider", "github", "--runtime", "static", "--app", "a"},
		"node, no start command":  {"--provider", "gitlab", "--runtime", "node", "--app", "a", "--deploy-target", "dist"},
		"runtime outside list":    {"--provider", "github", "--runtime", "node;printf x", "--app", "a", "--deploy-target", "d", "--start-command", "x"},
		"CLI too old":             {"--provider", "github", "--runtime", "static", "--app", "a", "--deploy-target", "d", "--cli-version", "v0.20.0"},
		"CLI version not a tag":   {"--provider", "github", "--runtime", "static", "--app", "a", "--deploy-target", "d", "--cli-version", "main"},
		"unknown provider":        {"--provider", "bitbucket", "--runtime", "static", "--app", "a", "--deploy-target", "d"},
		"app with space":          {"--provider", "github", "--runtime", "static", "--app", "a b", "--deploy-target", "d"},
		"github expression value": {"--provider", "github", "--runtime", "node", "--app", "a", "--deploy-target", "d", "--start-command", "${{ github.token }}"},
	}
	for name, argv := range cases {
		t.Run(name, func(t *testing.T) {
			var cap capture
			restore := setMockDeps(noRepoDeps(&cap))
			defer restore()
			if out, err := execCI(t, argv); err == nil {
				t.Fatalf("expected refusal\n%s", out)
			}
			if cap.called {
				t.Errorf("nothing may be written on refusal; wrote %s", cap.path)
			}
		})
	}
}

// h-cmzf.1: a runtime read from .hatch.toml goes through the same closed list.
func TestCI_RefusesUnknownRuntimeFromHatchToml(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".hatch.toml"), []byte("slug = \"a\"\nruntime = \"node; printf injected\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var cap capture
	restore := setMockDeps(&Deps{
		GitRemote: func() (string, error) { return "git@github.com:acme/repo.git", nil },
		Getwd:     func() (string, error) { return dir, nil },
		Stat:      func(string) bool { return false },
		WriteFile: cap.write,
	})
	defer restore()
	out, err := execCI(t, []string{"--deploy-target", "dist", "--start-command", "x"})
	if err == nil || !strings.Contains(err.Error(), "unsupported runtime") {
		t.Fatalf("err = %v, want unsupported runtime\n%s", err, out)
	}
	if cap.called {
		t.Error("nothing may be written")
	}
}

// h-cmzf.5: the CLI pin defaults to this binary's version, --cli-version
// overrides it, a dev build must name a release, and a failed checksum lookup
// writes nothing.
func TestCI_CLIVersionPin(t *testing.T) {
	var asked string
	var cap capture
	d := noRepoDeps(&cap)
	d.CLIVersion = func() string { return "dev" }
	d.FetchRelease = func(v string) (detect.CLIRelease, error) { asked = v; return fakeRelease(v) }
	restore := setMockDeps(d)
	defer restore()

	if _, err := execCI(t, args("--provider", "github", "--runtime", "node")); err == nil || !strings.Contains(err.Error(), "--cli-version") {
		t.Fatalf("dev build without --cli-version: err = %v", err)
	}
	if _, err := execCI(t, args("--provider", "github", "--runtime", "node", "--cli-version", "v0.23.1")); err != nil {
		t.Fatal(err)
	}
	if asked != "v0.23.1" || !strings.Contains(string(cap.content), `HATCH_CI_CLI_VERSION: "v0.23.1"`) {
		t.Errorf("pinned %q, workflow:\n%s", asked, cap.content)
	}

	var cap2 capture
	d2 := noRepoDeps(&cap2)
	d2.FetchRelease = func(string) (detect.CLIRelease, error) { return detect.CLIRelease{}, errors.New("HTTP 404") }
	restore2 := setMockDeps(d2)
	defer restore2()
	if _, err := execCI(t, args("--provider", "gitlab", "--runtime", "node")); err == nil {
		t.Fatal("expected the checksum lookup failure to abort")
	}
	if cap2.called {
		t.Error("nothing may be written when the CLI pin cannot be resolved")
	}
}

// The real ReadToken reads the first line of piped stdin, trimmed.
func TestReadTokenFromStdin_Piped(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = old }()
	if _, err := w.WriteString("  tok-from-pipe \nignored\n"); err != nil {
		t.Fatal(err)
	}
	w.Close()
	prompt := new(bytes.Buffer)
	got, err := readTokenFromStdin(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if got != "tok-from-pipe" {
		t.Errorf("got %q", got)
	}
	if prompt.Len() != 0 {
		t.Errorf("no prompt expected off a terminal, got %q", prompt.String())
	}
}

// The real Run hands stdin to the child process (and nothing in argv).
func TestDefaultRun_FeedsStdin(t *testing.T) {
	out := filepath.Join(t.TempDir(), "got")
	if err := defaultDeps().Run(strings.NewReader("via-stdin"), "sh", "-c", `cat > "$0"`, out); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "via-stdin" {
		t.Errorf("child stdin = %q", b)
	}
}
