package ci

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setMockDeps(d *Deps) func() {
	old := deps
	deps = d
	return func() { deps = old }
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
	cmd.SetArgs([]string{})
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
	cmd.SetArgs([]string{"--provider", "gitlab", "--runtime", "go"})
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
	cmd.SetArgs([]string{"--provider", "github", "--runtime", "node", "--print"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if cap.called {
		t.Errorf("--print must NOT write a file; wrote %q", cap.path)
	}
	if !strings.Contains(buf.String(), "hatch deploy") {
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
	cmd.SetArgs([]string{"--provider", "github", "--runtime", "node"})
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
	cmd2.SetArgs([]string{"--provider", "github", "--runtime", "node", "--yes"})
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
	cmd.SetArgs([]string{"--provider", "github", "--runtime", "static"})
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

// --set-secret --token runs the provider CLI via the injected runner.
func TestCI_SetSecret_RunsViaRunner(t *testing.T) {
	var ranBin string
	var ranArgs []string
	restore := setMockDeps(&Deps{
		GitRemote: func() (string, error) { return "git@github.com:acme/repo.git", nil },
		Getwd:     func() (string, error) { return "", os.ErrNotExist },
		Stat:      func(string) bool { return false },
		WriteFile: func(string, []byte) error { return nil },
		LookPath:  func(string) bool { return true },
		Run:       func(bin string, args ...string) error { ranBin = bin; ranArgs = args; return nil },
	})
	defer restore()
	cmd := NewCmd()
	cmd.SetArgs([]string{"--provider", "github", "--runtime", "node", "--token", "hatch_secret", "--set-secret"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if ranBin != "gh" {
		t.Errorf("ran %q, want gh", ranBin)
	}
	joined := strings.Join(ranArgs, " ")
	for _, want := range []string{"secret set HATCH_TOKEN", "hatch_secret", "acme/repo"} {
		if !strings.Contains(joined, want) {
			t.Errorf("gh args missing %q: %v", want, ranArgs)
		}
	}
	if !strings.Contains(buf.String(), "Set the HATCH_TOKEN secret") {
		t.Errorf("missing success message:\n%s", buf.String())
	}
}

// --set-secret with the provider CLI not installed falls back to manual instructions.
func TestCI_SetSecret_MissingCLI_Fallback(t *testing.T) {
	ran := false
	restore := setMockDeps(&Deps{
		GitRemote: func() (string, error) { return "git@github.com:acme/repo.git", nil },
		Getwd:     func() (string, error) { return "", os.ErrNotExist },
		Stat:      func(string) bool { return false },
		WriteFile: func(string, []byte) error { return nil },
		LookPath:  func(string) bool { return false },
		Run:       func(string, ...string) error { ran = true; return nil },
	})
	defer restore()
	cmd := NewCmd()
	cmd.SetArgs([]string{"--provider", "github", "--runtime", "node", "--token", "hatch_secret", "--set-secret"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if ran {
		t.Error("must not run when the provider CLI is not installed")
	}
	if !strings.Contains(buf.String(), "not installed") {
		t.Errorf("missing not-installed fallback:\n%s", buf.String())
	}
}
