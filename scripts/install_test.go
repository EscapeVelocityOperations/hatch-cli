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
	if !strings.Contains(stdout, "Checksum verified") {
		t.Errorf("expected stdout to confirm checksum verification, got:\n%s", stdout)
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

func TestInstallSh_Syntax(t *testing.T) {
	if err := exec.Command("sh", "-n", "install.sh").Run(); err != nil {
		t.Fatalf("sh -n install.sh: %v", err)
	}
}
