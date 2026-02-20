package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	checkInterval = 4 * time.Hour
	httpTimeout   = 5 * time.Second
	releasesURL   = "https://api.github.com/repos/EscapeVelocityOperations/hatch-cli/releases/latest"
	cacheFileName = "update-check.json"
)

// CheckResult holds the result of an update check.
type CheckResult struct {
	LatestVersion string `json:"latest_version"`
	CurrentVersion string `json:"-"`
	ReleaseURL    string `json:"release_url"`
	UpdateAvailable bool `json:"-"`
}

type cacheEntry struct {
	LatestVersion string `json:"latest_version"`
	CheckedAt     string `json:"checked_at"`
	ReleaseURL    string `json:"release_url"`
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// Check queries GitHub for the latest release and returns a result
// if an update is available. Returns nil if current version is up-to-date
// or on any error (fails silently).
func Check(currentVersion string) *CheckResult {
	if currentVersion == "" || currentVersion == "dev" {
		return nil
	}

	// Check cache first
	if cached := readCache(); cached != nil {
		checkedAt, err := time.Parse(time.RFC3339, cached.CheckedAt)
		if err == nil && time.Since(checkedAt) < checkInterval {
			if compareVersions(cached.LatestVersion, currentVersion) > 0 {
				return &CheckResult{
					LatestVersion:   cached.LatestVersion,
					CurrentVersion:  currentVersion,
					ReleaseURL:      cached.ReleaseURL,
					UpdateAvailable: true,
				}
			}
			return nil
		}
	}

	// Fetch from GitHub
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Get(releasesURL)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")

	// Write cache
	writeCache(&cacheEntry{
		LatestVersion: latestVersion,
		CheckedAt:     time.Now().UTC().Format(time.RFC3339),
		ReleaseURL:    release.HTMLURL,
	})

	if compareVersions(latestVersion, currentVersion) > 0 {
		return &CheckResult{
			LatestVersion:   latestVersion,
			CurrentVersion:  currentVersion,
			ReleaseURL:      release.HTMLURL,
			UpdateAvailable: true,
		}
	}

	return nil
}

// FormatNotification returns a formatted update notification string.
func FormatNotification(r *CheckResult) string {
	if r == nil || !r.UpdateAvailable {
		return ""
	}
	return fmt.Sprintf("\n  A new version of hatch is available: %s -> %s\n  Run: curl -fsSL https://gethatch.eu/install | sh\n",
		r.CurrentVersion, r.LatestVersion)
}

// compareVersions compares two semver strings.
// Returns >0 if a > b, 0 if equal, <0 if a < b.
func compareVersions(a, b string) int {
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")

	partsA := strings.SplitN(a, ".", 3)
	partsB := strings.SplitN(b, ".", 3)

	for i := 0; i < 3; i++ {
		var va, vb int
		if i < len(partsA) {
			va, _ = strconv.Atoi(partsA[i])
		}
		if i < len(partsB) {
			vb, _ = strconv.Atoi(partsB[i])
		}
		if va != vb {
			return va - vb
		}
	}
	return 0
}

func cacheFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".hatch", cacheFileName)
}

func readCache() *cacheEntry {
	path := cacheFilePath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil
	}
	return &entry
}

func writeCache(entry *cacheEntry) {
	path := cacheFilePath()
	if path == "" {
		return
	}
	dir := filepath.Dir(path)
	_ = os.MkdirAll(dir, 0700)
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0600)
}
