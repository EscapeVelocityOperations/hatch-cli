package api

import (
	"strings"
	"testing"
)

// TestCheckArtifactSize pins the client-side limit to the server's: 100 MB
// (h-yqo6k). The CLI used to allow 500 MB, so a 100-500 MB artifact was
// uploaded, cut at 100 MB by hatch-api, and failed to build.
func TestCheckArtifactSize(t *testing.T) {
	if MaxArtifactBytes != 100*1024*1024 {
		t.Fatalf("MaxArtifactBytes = %d, want 104857600 (hatch-api/hatch-control limit)", MaxArtifactBytes)
	}
	if err := CheckArtifactSize(0); err != nil {
		t.Errorf("empty artifact: %v", err)
	}
	if err := CheckArtifactSize(MaxArtifactBytes); err != nil {
		t.Errorf("artifact of exactly the limit refused: %v", err)
	}
	err := CheckArtifactSize(MaxArtifactBytes + 1)
	if err == nil {
		t.Fatal("artifact one byte over the limit accepted")
	}
	if want := "artifact is 100.1 MB, maximum is 100 MB"; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q, want it to contain %q", err, want)
	}
	err = CheckArtifactSize(150 * 1024 * 1024)
	if err == nil || !strings.Contains(err.Error(), "artifact is 150 MB, maximum is 100 MB") {
		t.Errorf("150 MB artifact: error %v, want the size and the limit", err)
	}
	if !strings.Contains(err.Error(), ".hatchignore") {
		t.Errorf("error %q does not point at .hatchignore", err)
	}
}

func TestFormatArtifactMB(t *testing.T) {
	cases := map[int64]string{
		MaxArtifactBytes:      "100 MB",
		MaxArtifactBytes + 1:  "100.1 MB",
		512*1024*1024 + 1<<19: "512.5 MB",
	}
	for n, want := range cases {
		if got := FormatArtifactMB(n); got != want {
			t.Errorf("FormatArtifactMB(%d) = %q, want %q", n, got, want)
		}
	}
}
