// Package scripts hosts Go-driven behavior tests for the shell scripts in
// this directory (T-018/T-018: scripts/hatch-mcp.sh, the plugin's MCP
// server bootstrap wrapper, D4).
package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeStub creates an executable POSIX shell script at dir/name.
func writeStub(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	full := "#!/bin/sh\n" + body
	if err := os.WriteFile(path, []byte(full), 0755); err != nil {
		t.Fatalf("writing stub %s: %v", name, err)
	}
	return path
}

// runWrapper execs scripts/hatch-mcp.sh with an isolated HOME and a PATH
// containing only stubBin (plus the real system dirs sh needs for its own
// builtins — mkdir/chmod/mktemp/rm — none of which are named "hatch").
//
// systemBinDir is REQUIRED (not defaulted) and always wins over the
// wrapper's real default: this dev machine (and many others) has a real
// hatch binary at the literal /usr/local/bin the wrapper falls back to, so
// every "absent" test must point HATCH_SYSTEM_BIN_DIR at an isolated temp
// dir or it will silently pick up that real binary instead of the stub.
func runWrapper(t *testing.T, stubBin, home, systemBinDir string) (stdout, stderr string, exitErr error) {
	t.Helper()
	cmd := exec.Command("sh", "hatch-mcp.sh")
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + stubBin + ":/bin:/usr/bin",
		"HATCH_SYSTEM_BIN_DIR=" + systemBinDir,
	}

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

func TestHatchMCPWrapper_Syntax(t *testing.T) {
	if err := exec.Command("sh", "-n", "hatch-mcp.sh").Run(); err != nil {
		t.Fatalf("sh -n hatch-mcp.sh: %v", err)
	}
}

// (a) binary on PATH -> execs `hatch mcp`, no bootstrap.
func TestHatchMCPWrapper_BinaryOnPath_ExecsHatchMCP(t *testing.T) {
	stubBin := t.TempDir()
	home := t.TempDir()
	writeStub(t, stubBin, "hatch", `echo "STUB_HATCH args=$*"`+"\n")
	// No curl stub on PATH: if the wrapper tried to bootstrap, "command not
	// found" would surface in stderr and the run would fail.

	stdout, stderr, err := runWrapper(t, stubBin, home, t.TempDir())
	if err != nil {
		t.Fatalf("wrapper failed: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "STUB_HATCH args=mcp") {
		t.Errorf("expected the wrapper to exec the on-PATH stub with 'mcp', got stdout: %q", stdout)
	}
	if strings.Contains(stderr, "bootstrapping") {
		t.Errorf("expected no bootstrap when hatch was found on PATH, got stderr: %q", stderr)
	}
}

// (b) absent -> bootstraps from the pinned URL with HATCH_INSTALL_DIR set,
// then execs the freshly-installed stub.
func TestHatchMCPWrapper_Absent_BootstrapsThenExecs(t *testing.T) {
	stubBin := t.TempDir()
	home := t.TempDir()

	// Stub curl: records the requested URL, and "installs" a stub hatch
	// binary into $HATCH_INSTALL_DIR by writing an installer script that
	// the wrapper then runs via `sh`.
	writeStub(t, stubBin, "curl", `
url=""
out=""
while [ $# -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    -*) shift ;;
    *) url="$1"; shift ;;
  esac
done
echo "$url" > `+filepath.Join(home, "curl_url.txt")+`
cat > "$out" <<'INSTALLER'
#!/bin/sh
mkdir -p "$HATCH_INSTALL_DIR"
cat > "$HATCH_INSTALL_DIR/hatch" <<'STUB'
#!/bin/sh
echo "STUB_HATCH args=$*"
STUB
chmod +x "$HATCH_INSTALL_DIR/hatch"
INSTALLER
`)

	stdout, stderr, err := runWrapper(t, stubBin, home, t.TempDir())
	if err != nil {
		t.Fatalf("wrapper failed: %v\nstderr: %s", err, stderr)
	}

	gotURL, readErr := os.ReadFile(filepath.Join(home, "curl_url.txt"))
	if readErr != nil {
		t.Fatalf("expected curl to have been invoked, no URL recorded: %v", readErr)
	}
	if strings.TrimSpace(string(gotURL)) != "https://gethatch.eu/install" {
		t.Errorf("expected the pinned install URL, got: %q", strings.TrimSpace(string(gotURL)))
	}
	if !strings.Contains(stdout, "STUB_HATCH args=mcp") {
		t.Errorf("expected the wrapper to exec the newly-installed stub with 'mcp', got stdout: %q", stdout)
	}
	if !strings.Contains(stderr, "bootstrapping") {
		t.Errorf("expected a bootstrap notice on stderr, got: %q", stderr)
	}
}

// (c) installer failure -> non-zero exit, no exec, clear stderr message.
func TestHatchMCPWrapper_InstallerFailure_AbortsNonZero(t *testing.T) {
	stubBin := t.TempDir()
	home := t.TempDir()
	writeStub(t, stubBin, "curl", `exit 1`+"\n")

	stdout, stderr, err := runWrapper(t, stubBin, home, t.TempDir())
	if err == nil {
		t.Fatal("expected the wrapper to exit non-zero when the installer download fails")
	}
	if stdout != "" {
		t.Errorf("expected no exec (empty stdout) on installer failure, got: %q", stdout)
	}
	if !strings.Contains(stderr, "hatch-mcp.sh") {
		t.Errorf("expected a clear hatch-mcp.sh error message on stderr, got: %q", stderr)
	}
}

// A binary found via the fixed fallback paths (not PATH) must also skip
// bootstrap — exercises the $HOME/.hatch/bin resolution step.
func TestHatchMCPWrapper_FoundInHatchBinDir_SkipsBootstrap(t *testing.T) {
	stubBin := t.TempDir() // empty: nothing named "hatch" on PATH
	home := t.TempDir()
	hatchBinDir := filepath.Join(home, ".hatch", "bin")
	if err := os.MkdirAll(hatchBinDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeStub(t, hatchBinDir, "hatch", `echo "STUB_HATCH args=$*"`+"\n")

	stdout, stderr, err := runWrapper(t, stubBin, home, t.TempDir())
	if err != nil {
		t.Fatalf("wrapper failed: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "STUB_HATCH args=mcp") {
		t.Errorf("expected the wrapper to exec the ~/.hatch/bin stub with 'mcp', got stdout: %q", stdout)
	}
	if strings.Contains(stderr, "bootstrapping") {
		t.Errorf("expected no bootstrap when hatch was found in ~/.hatch/bin, got stderr: %q", stderr)
	}
}

// The 4th fallback (a fixed system bin dir) is only testable via the
// HATCH_SYSTEM_BIN_DIR override — the wrapper's real default is the literal
// /usr/local/bin, which this dev machine (and many others) has a real hatch
// binary in, making that path untestable without touching the real
// filesystem. HATCH_SYSTEM_BIN_DIR is a test-only seam; production callers
// never set it, so the default stays /usr/local/bin.
func TestHatchMCPWrapper_FoundInSystemBinDir_SkipsBootstrap(t *testing.T) {
	stubBin := t.TempDir() // empty: nothing named "hatch" on PATH
	home := t.TempDir()
	systemBinDir := t.TempDir()
	writeStub(t, systemBinDir, "hatch", `echo "STUB_HATCH args=$*"`+"\n")

	stdout, stderr, err := runWrapper(t, stubBin, home, systemBinDir)
	if err != nil {
		t.Fatalf("wrapper failed: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "STUB_HATCH args=mcp") {
		t.Errorf("expected the wrapper to exec the system-bin-dir stub with 'mcp', got stdout: %q", stdout)
	}
	if strings.Contains(stderr, "bootstrapping") {
		t.Errorf("expected no bootstrap when hatch was found in the system bin dir, got stderr: %q", stderr)
	}
}
