package api

import (
	"strings"
	"testing"
)

const (
	mib = int64(1) << 20
	gib = int64(1) << 30
)

// TestCheckArtifactSize pins the client-side limit to the server's: 1 GiB
// (h-5ghe9, raised from 100 MB; h-yqo6k before it: the CLI used to allow more
// than the server did, so such an artifact was uploaded, cut, and failed to
// build).
func TestCheckArtifactSize(t *testing.T) {
	if MaxArtifactBytes != 1073741824 {
		t.Fatalf("MaxArtifactBytes = %d, want 1073741824 (hatch-api/hatch-control limit)", int64(MaxArtifactBytes))
	}
	for name, n := range map[string]int64{
		"empty":             0,
		"the old 100 MB":    100 * mib,
		"999 MiB":           999 * mib,
		"exactly the limit": MaxArtifactBytes,
	} {
		if err := CheckArtifactSize(n); err != nil {
			t.Errorf("%s: artifact of %d bytes refused: %v", name, n, err)
		}
	}

	err := CheckArtifactSize(MaxArtifactBytes + 1)
	if err == nil {
		t.Fatal("artifact one byte over the limit accepted")
	}
	want := "artifact is 1.1 GB, maximum is 1 GB: exclude build caches, sources or media with a .hatchignore file"
	if err.Error() != want {
		t.Errorf("error %q, want %q", err, want)
	}

	err = CheckArtifactSize(2 * gib)
	if err == nil || !strings.Contains(err.Error(), "artifact is 2 GB, maximum is 1 GB") {
		t.Errorf("2 GiB artifact: error %v, want the size and the limit", err)
	}
	// Sizes beyond what a 32-bit int holds: CheckArtifactSize takes int64.
	if err := CheckArtifactSize(5 * gib); err == nil || !strings.Contains(err.Error(), "artifact is 5 GB") {
		t.Errorf("5 GiB artifact: error %v", err)
	}
}

func TestFormatArtifactSize(t *testing.T) {
	cases := map[int64]string{
		0:                    "0 MB",
		100 * mib:            "100 MB",
		100*mib + 1:          "100.1 MB", // rounded up: never prints as the size
		512*mib + mib/2:      "512.5 MB",
		1023 * mib:           "1023 MB",
		gib - 1:              "1024 MB", // still MB below 1 GiB, rounded up
		MaxArtifactBytes:     "1 GB",
		MaxArtifactBytes + 1: "1.1 GB", // rounded up: never prints as the limit
		gib + gib/2:          "1.5 GB",
		2 * gib:              "2 GB",
	}
	for n, want := range cases {
		if got := FormatArtifactSize(n); got != want {
			t.Errorf("FormatArtifactSize(%d) = %q, want %q", n, got, want)
		}
	}
}
