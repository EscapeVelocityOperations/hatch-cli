package ci

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// MinCIVersion is the oldest Hatch CLI release a generated workflow may pin: the
// workflow calls `hatch deploy --app` and relies on HATCH_ACCEPT_TERMS, both
// introduced after v0.20.0 (h-cmzf.2, h-cmzf.4).
const MinCIVersion = "v0.21.0"

// ChecksumsURL is where a release publishes the sha256 of its binaries.
func ChecksumsURL(version string) string {
	return "https://github.com/" + CLIRepo + "/releases/download/" + version + "/checksums.txt"
}

// ParseChecksums reads a `sha256sum`-format checksums.txt into the CLI pin for
// version. It fails closed when either linux binary has no valid entry.
func ParseChecksums(version string, r io.Reader) (CLIRelease, error) {
	rel := CLIRelease{Version: version}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 2 {
			continue
		}
		sum, name := strings.ToLower(f[0]), strings.TrimPrefix(f[1], "*")
		switch name {
		case "hatch-linux-amd64":
			rel.SHA256AMD64 = sum
		case "hatch-linux-arm64":
			rel.SHA256ARM64 = sum
		}
	}
	if err := sc.Err(); err != nil {
		return CLIRelease{}, fmt.Errorf("reading checksums for %s: %w", version, err)
	}
	if !sha256Re.MatchString(rel.SHA256AMD64) || !sha256Re.MatchString(rel.SHA256ARM64) {
		return CLIRelease{}, fmt.Errorf("checksums.txt for %s lacks valid hatch-linux-amd64/hatch-linux-arm64 entries", version)
	}
	return rel, nil
}

// FetchCLIRelease downloads the checksums of a published release over HTTPS.
// The resulting pin is frozen into the generated (and reviewed) workflow.
func FetchCLIRelease(version string) (CLIRelease, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(ChecksumsURL(version))
	if err != nil {
		return CLIRelease{}, fmt.Errorf("fetching checksums for Hatch CLI %s: %w", version, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return CLIRelease{}, fmt.Errorf("fetching checksums for Hatch CLI %s: HTTP %d (is %s a published release?)", version, resp.StatusCode, version)
	}
	return ParseChecksums(version, io.LimitReader(resp.Body, 1<<20))
}

// CheckCIVersion refuses a CLI version that is not a release tag, or one older
// than MinCIVersion.
func CheckCIVersion(version string) error {
	if !cliVersionRe.MatchString(version) {
		return fmt.Errorf("cannot pin Hatch CLI version %q in a workflow: pass --cli-version vX.Y.Z (a published release ≥ %s)", version, MinCIVersion)
	}
	if compareSemver(version, MinCIVersion) < 0 {
		return fmt.Errorf("Hatch CLI %s is too old for generated workflows (needs ≥ %s for deploy --app and HATCH_ACCEPT_TERMS)", version, MinCIVersion)
	}
	return nil
}

// compareSemver compares two vX.Y.Z tags already matched by cliVersionRe.
func compareSemver(a, b string) int {
	pa := strings.Split(strings.TrimPrefix(a, "v"), ".")
	pb := strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < 3; i++ {
		x, _ := strconv.Atoi(pa[i])
		y, _ := strconv.Atoi(pb[i])
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}
