package ci

import (
	"bytes"
	"errors"
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
