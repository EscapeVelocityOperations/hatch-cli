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

// h-tymh: `hatch ci` detects the provider (from the origin remote) and runtime
// (from project signature files) and prints a PLAN — provider, runtime, the files
// it would generate, and the HATCH_TOKEN secret name. It never writes or pushes.
func TestCI_PrintsPlan(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	restore := setMockDeps(&Deps{
		GitRemote: func() (string, error) { return "git@github.com:acme/repo.git", nil },
		Getwd:     func() (string, error) { return dir, nil },
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

	out := buf.String()
	for _, want := range []string{"github", "node", "HATCH_TOKEN", ".github/workflows/hatch-deploy.yml"} {
		if !strings.Contains(out, want) {
			t.Errorf("plan output missing %q\noutput:\n%s", want, out)
		}
	}
}

// --provider / --runtime overrides bypass detection (so a non-repo dir still works).
func TestCI_ProviderAndRuntimeOverride(t *testing.T) {
	restore := setMockDeps(&Deps{
		GitRemote: func() (string, error) { return "", os.ErrNotExist },
		Getwd:     func() (string, error) { return "", os.ErrNotExist },
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

	out := buf.String()
	if !strings.Contains(out, "gitlab") || !strings.Contains(out, "go") || !strings.Contains(out, ".gitlab-ci.yml") {
		t.Errorf("override plan wrong:\n%s", out)
	}
}
