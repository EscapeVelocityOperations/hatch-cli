package volume

import (
	"errors"
	"strings"
	"testing"
)

// h-gcf5h TDD-red (h-poo35): `hatch volume enable|status|disable` behavior
// against injected deps, following the domain cmd test pattern.

func TestNewCmd_Structure(t *testing.T) {
	cmd := NewCmd()
	if cmd.Use != "volume" {
		t.Fatalf("Use = %q, want volume", cmd.Use)
	}
	want := map[string]bool{"enable": false, "status": false, "disable": false}
	for _, sub := range cmd.Commands() {
		name := strings.Fields(sub.Use)[0]
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("subcommand %q missing", name)
		}
	}
}

func TestRunEnable_NotLoggedIn(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "", errors.New("no token") },
	}
	defer func() { deps = defaultDeps() }()

	err := runEnable("test-app", 1024)
	if err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("err = %v, want 'not logged in'", err)
	}
}

func TestRunEnable_PassesSlugAndSize(t *testing.T) {
	var gotSlug string
	var gotSize int
	deps = &Deps{
		GetToken: func() (string, error) { return "tok", nil },
		EnableVolume: func(token, slug string, sizeMB int) error {
			gotSlug, gotSize = slug, sizeMB
			return nil
		},
	}
	defer func() { deps = defaultDeps() }()

	if err := runEnable("test-app", 2048); err != nil {
		t.Fatalf("runEnable: %v", err)
	}
	if gotSlug != "test-app" || gotSize != 2048 {
		t.Fatalf("EnableVolume called with (%q, %d), want (test-app, 2048)", gotSlug, gotSize)
	}
}

func TestRunStatus_FetchesVolume(t *testing.T) {
	var gotSlug string
	deps = &Deps{
		GetToken: func() (string, error) { return "tok", nil },
		GetVolume: func(token, slug string) (VolumeInfo, error) {
			gotSlug = slug
			return VolumeInfo{SizeMB: 1024, UsedMB: 12, Mount: "/data", Status: "active"}, nil
		},
	}
	defer func() { deps = defaultDeps() }()

	if err := runStatus("test-app"); err != nil {
		t.Fatalf("runStatus: %v", err)
	}
	if gotSlug != "test-app" {
		t.Fatalf("GetVolume called with %q, want test-app", gotSlug)
	}
}

func TestRunDisable_GraceByDefault(t *testing.T) {
	var gotNow bool
	called := false
	deps = &Deps{
		GetToken: func() (string, error) { return "tok", nil },
		DisableVolume: func(token, slug string, now bool) error {
			called = true
			gotNow = now
			return nil
		},
	}
	defer func() { deps = defaultDeps() }()

	if err := runDisable("test-app", false); err != nil {
		t.Fatalf("runDisable: %v", err)
	}
	if !called || gotNow {
		t.Fatalf("DisableVolume(called=%v, now=%v), want called with now=false (grace)", called, gotNow)
	}
}

func TestRunDisable_NowFlag(t *testing.T) {
	var gotNow bool
	deps = &Deps{
		GetToken: func() (string, error) { return "tok", nil },
		DisableVolume: func(token, slug string, now bool) error {
			gotNow = now
			return nil
		},
	}
	defer func() { deps = defaultDeps() }()

	if err := runDisable("test-app", true); err != nil {
		t.Fatalf("runDisable --now: %v", err)
	}
	if !gotNow {
		t.Fatal("DisableVolume called with now=false, want now=true")
	}
}
