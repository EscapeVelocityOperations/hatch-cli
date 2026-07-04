// Package scripts also tests scripts/install.sh's checksum verification
// (T-022, D5): a good sha256 entry installs, a mismatched one aborts
// non-zero without installing, and an unfetchable checksums.txt aborts
// before any install happens too — fail-closed, matching the wrapper's
// stance in hatch_mcp_test.go.
package scripts

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// curlStubBody serves fixed binary/checksums content regardless of URL,
// branching on whether the requested URL ends in checksums.txt. It mirrors
// the `curl -fsSL -o <path> <url>` shape both download_binary() call sites
// in install.sh use, so $3 is the output path and $4 is the URL.
const curlStubBody = `outfile="$3"
url="$4"
case "$url" in
	*/checksums.txt)
		if [ -n "${STUB_CHECKSUMS_MISSING:-}" ]; then
			exit 22
		fi
		printf '%s' "$STUB_CHECKSUMS_CONTENT" > "$outfile"
		;;
	*)
		printf '%s' "$STUB_BINARY_CONTENT" > "$outfile"
		;;
esac
`

// releaseFilename mirrors install.sh's detect_platform() normalization,
// which already speaks Go's runtime.GOOS/GOARCH vocabulary (linux/darwin,
// amd64/arm64) — so the expected release filename matches on whatever
// platform runs the test.
func releaseFilename() string {
	return fmt.Sprintf("hatch-%s-%s", runtime.GOOS, runtime.GOARCH)
}

func sha256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", sum)
}

// runInstaller execs scripts/install.sh with an isolated HOME and install
// dir, and a stub curl. HATCH_INSTALL_DIR skips the interactive
// install-location prompt; listing installDir in PATH skips the "add to
// PATH?" prompt; SHELL=/bin/sh (not zsh) skips the completions prompt — so
// the whole script runs to completion non-interactively, no stdin needed.
func runInstaller(t *testing.T, stubBin, home, installDir, binaryContent, checksumsContent string, checksumsMissing bool) (stdout, stderr string, exitErr error) {
	t.Helper()
	writeStub(t, stubBin, "curl", curlStubBody)

	cmd := exec.Command("sh", "install.sh")
	env := []string{
		"HOME=" + home,
		"PATH=" + stubBin + ":" + installDir + ":/bin:/usr/bin",
		"HATCH_INSTALL_DIR=" + installDir,
		"SHELL=/bin/sh",
		"STUB_BINARY_CONTENT=" + binaryContent,
		"STUB_CHECKSUMS_CONTENT=" + checksumsContent,
	}
	if checksumsMissing {
		env = append(env, "STUB_CHECKSUMS_MISSING=1")
	}
	cmd.Env = env

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

func TestInstallSh_GoodChecksum_Installs(t *testing.T) {
	stubBin := t.TempDir()
	home := t.TempDir()
	installDir := t.TempDir()

	binary := "#!/bin/sh\necho ok\n"
	checksums := fmt.Sprintf("%s  %s\n", sha256Hex(binary), releaseFilename())

	stdout, stderr, err := runInstaller(t, stubBin, home, installDir, binary, checksums, false)
	if err != nil {
		t.Fatalf("install.sh failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	installed := filepath.Join(installDir, "hatch")
	info, statErr := os.Stat(installed)
	if statErr != nil {
		t.Fatalf("expected %s to exist: %v", installed, statErr)
	}
	if info.Mode()&0111 == 0 {
		t.Errorf("installed binary %s is not executable", installed)
	}
	if !strings.Contains(stderr, "Checksum verified") {
		t.Errorf("expected stderr to confirm checksum verification, got:\n%s", stderr)
	}
}

// TestInstallSh_StdoutStaysClean reproduces the real D4 bootstrap-via-MCP
// scenario one level further than TestInstallSh_NonInteractive_DoesNotHangOnOpenStdin:
// hatch-mcp.sh runs `sh "$tmp_installer"` with stdout INHERITED — during a
// real MCP-subprocess bootstrap, that stdout IS the live MCP JSON-RPC
// channel Claude Code is reading from. install.sh's info()/warn()/ok()/dim()
// helpers print with a bare `printf` (no `>&2`), so its entire installer
// banner/progress UI lands on stdout — which would corrupt the MCP
// handshake for every single first-time bootstrap, the exact case D4/D5
// exist to support. This was invisible to the other tests in this file:
// they check specific substrings are present somewhere in stdout/stderr,
// never which fd carries them.
func TestInstallSh_StdoutStaysClean(t *testing.T) {
	stubBin := t.TempDir()
	home := t.TempDir()
	installDir := t.TempDir()

	binary := "#!/bin/sh\necho ok\n"
	checksums := fmt.Sprintf("%s  %s\n", sha256Hex(binary), releaseFilename())

	stdout, _, err := runInstaller(t, stubBin, home, installDir, binary, checksums, false)
	if err != nil {
		t.Fatalf("install.sh failed: %v", err)
	}

	if strings.TrimSpace(stdout) != "" {
		t.Errorf("install.sh wrote to stdout — this would corrupt the MCP stdio channel during a real bootstrap. stdout:\n%s", stdout)
	}
}

func TestInstallSh_BadChecksum_AbortsWithoutInstalling(t *testing.T) {
	stubBin := t.TempDir()
	home := t.TempDir()
	installDir := t.TempDir()

	binary := "#!/bin/sh\necho ok\n"
	checksums := fmt.Sprintf("%s  %s\n", sha256Hex("not the real binary"), releaseFilename())

	_, stderr, err := runInstaller(t, stubBin, home, installDir, binary, checksums, false)
	if err == nil {
		t.Fatalf("expected install.sh to fail on checksum mismatch, it exited 0")
	}
	if !strings.Contains(stderr, "checksum mismatch") {
		t.Errorf("expected stderr to mention checksum mismatch, got:\n%s", stderr)
	}
	if _, statErr := os.Stat(filepath.Join(installDir, "hatch")); statErr == nil {
		t.Errorf("binary must not be installed when checksum verification fails")
	}
}

func TestInstallSh_MissingChecksums_Aborts(t *testing.T) {
	stubBin := t.TempDir()
	home := t.TempDir()
	installDir := t.TempDir()

	binary := "#!/bin/sh\necho ok\n"

	_, _, err := runInstaller(t, stubBin, home, installDir, binary, "", true)
	if err == nil {
		t.Fatalf("expected install.sh to fail when checksums.txt can't be fetched, it exited 0")
	}
	if _, statErr := os.Stat(filepath.Join(installDir, "hatch")); statErr == nil {
		t.Errorf("binary must not be installed when checksums.txt cannot be fetched")
	}
}

// TestInstallSh_NonInteractive_DoesNotHangOnOpenStdin reproduces the real
// D4 bootstrap path: hatch-mcp.sh execs `sh "$tmp_installer"` with stdin
// INHERITED, not redirected — when the wrapper runs as an MCP server
// subprocess, that inherited stdin is the live MCP stdio stream, not a
// closed/EOF pipe. PATH deliberately excludes installDir here so
// check_path's "not in PATH, add it now? [Y/n]" prompt is reached (unlike
// the other tests in this file, which sidestep every prompt). Stdin is an
// open, never-written, never-closed pipe: a real TTY-less `ask()` call
// would call `read -r answer` and block on it forever (or, worse, steal a
// line of live MCP protocol traffic in production). A safe implementation
// must detect the non-interactive stdin up front and skip straight to the
// default without ever reading.
//
// Scope note: this only asserts against the hang/steal risk. The guard
// still returns the SAME "y" default a human would get by pressing Enter
// (so check_path may still append a PATH export to the rc file) — that's
// the behavior this port intentionally preserves from the source it's
// synced from (hatch-landing/public/install); silencing that default
// specifically for non-interactive runs is a separate, lower-severity
// follow-up, not this fix.
func TestInstallSh_NonInteractive_DoesNotHangOnOpenStdin(t *testing.T) {
	stubBin := t.TempDir()
	home := t.TempDir()
	installDir := t.TempDir() // fresh dir, deliberately NOT added to PATH below

	writeStub(t, stubBin, "curl", curlStubBody)

	binary := "#!/bin/sh\necho ok\n"
	checksums := fmt.Sprintf("%s  %s\n", sha256Hex(binary), releaseFilename())

	// A real OS pipe (*os.File on both ends), not io.Pipe(): when Stdin is
	// an *os.File, Go connects the child's fd 0 to it directly and Wait()
	// only waits on the CHILD process exiting. io.Pipe()'s Reader is a
	// plain io.Reader, which instead makes Go spawn a background copy
	// goroutine that Wait() also blocks on — that goroutine would block
	// forever on our never-written, never-closed write end regardless of
	// whether the child already exited, which would make this test hang
	// on its OWN plumbing rather than on anything install.sh does.
	pr, pw, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("os.Pipe: %v", pipeErr)
	}
	defer pr.Close()
	defer pw.Close()

	cmd := exec.Command("sh", "install.sh")
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + stubBin + ":/bin:/usr/bin", // installDir NOT included
		"HATCH_INSTALL_DIR=" + installDir,
		"SHELL=/bin/sh",
		"STUB_BINARY_CONTENT=" + binary,
		"STUB_CHECKSUMS_CONTENT=" + checksums,
	}
	cmd.Stdin = pr

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("install.sh exited with error: %v\nstdout:\n%s\nstderr:\n%s", err, outBuf.String(), errBuf.String())
		}
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("install.sh hung instead of detecting non-interactive stdin and defaulting silently\nstdout so far:\n%s\nstderr so far:\n%s", outBuf.String(), errBuf.String())
	}
}

func TestInstallSh_Syntax(t *testing.T) {
	if err := exec.Command("sh", "-n", "install.sh").Run(); err != nil {
		t.Fatalf("sh -n install.sh: %v", err)
	}
}
