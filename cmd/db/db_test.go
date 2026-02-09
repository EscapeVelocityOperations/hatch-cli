package db

import (
	"fmt"
	"net"
	"strings"
	"testing"
)

func TestRunConnect_NotLoggedIn(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "", nil },
	}
	defer func() { deps = defaultDeps() }()

	err := runConnect(nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunConnect_NoRemoteNoArg(t *testing.T) {
	deps = &Deps{
		GetToken:  func() (string, error) { return "tok123", nil },
		HasRemote: func(name string) bool { return false },
	}
	defer func() { deps = defaultDeps() }()

	err := runConnect(nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no egg specified") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunConnect_ListenError(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		Listen: func(network, address string) (net.Listener, error) {
			return nil, fmt.Errorf("address already in use")
		},
	}
	defer func() { deps = defaultDeps() }()

	err := runConnect(nil, []string{"myapp"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveSlug_WithArg(t *testing.T) {
	slug, err := resolveSlug([]string{"myapp"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if slug != "myapp" {
		t.Fatalf("expected slug 'myapp', got %q", slug)
	}
}

func TestResolveSlug_AutoDetect(t *testing.T) {
	deps = &Deps{
		HasRemote:    func(name string) bool { return true },
		GetRemoteURL: func(name string) (string, error) { return "https://tok@git.gethatch.eu/deploy/detected.git", nil },
	}
	defer func() { deps = defaultDeps() }()

	slug, err := resolveSlug(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if slug != "detected" {
		t.Fatalf("expected slug 'detected', got %q", slug)
	}
}

func TestResolveSlug_NoRemoteNoArg(t *testing.T) {
	deps = &Deps{
		HasRemote: func(name string) bool { return false },
	}
	defer func() { deps = defaultDeps() }()

	_, err := resolveSlug(nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no egg specified") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveSlug_RemoteError(t *testing.T) {
	deps = &Deps{
		HasRemote:    func(name string) bool { return true },
		GetRemoteURL: func(name string) (string, error) { return "", fmt.Errorf("git error") },
	}
	defer func() { deps = defaultDeps() }()

	_, err := resolveSlug(nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "git error") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWSURLForSlug(t *testing.T) {
	url := wsURLForSlug("myapp")
	expected := "wss://api.gethatch.eu/v1/apps/myapp/db/tunnel"
	if url != expected {
		t.Fatalf("expected URL %q, got %q", expected, url)
	}
}

func TestWSURLForSlug_EdgeCases(t *testing.T) {
	tests := []struct {
		slug     string
		expected string
	}{
		{"test", "wss://api.gethatch.eu/v1/apps/test/db/tunnel"},
		{"my-app-123", "wss://api.gethatch.eu/v1/apps/my-app-123/db/tunnel"},
		{"app_with_underscore", "wss://api.gethatch.eu/v1/apps/app_with_underscore/db/tunnel"},
	}
	for _, tt := range tests {
		t.Run(tt.slug, func(t *testing.T) {
			url := wsURLForSlug(tt.slug)
			if url != tt.expected {
				t.Fatalf("expected URL %q, got %q", tt.expected, url)
			}
		})
	}
}

func TestNewConnectCmd(t *testing.T) {
	cmd := newConnectCmd()
	if cmd.Use != "connect [slug] [-- psql-args...]" {
		t.Fatalf("unexpected use: %s", cmd.Use)
	}
	if cmd.Short != "Open a local TCP proxy to your egg's database" {
		t.Fatalf("unexpected short: %s", cmd.Short)
	}
	portFlag := cmd.Flags().Lookup("port")
	if portFlag == nil || portFlag.DefValue != "15432" {
		t.Fatalf("unexpected default port")
	}
	hostFlag := cmd.Flags().Lookup("host")
	if hostFlag == nil || hostFlag.DefValue != "localhost" {
		t.Fatalf("unexpected default host")
	}
	noPsqlFlag := cmd.Flags().Lookup("no-psql")
	if noPsqlFlag == nil {
		t.Fatal("no-psql flag not found")
	}
}

func TestNewCmd(t *testing.T) {
	cmd := NewCmd()
	if cmd.Use != "db" {
		t.Fatalf("unexpected use: %s", cmd.Use)
	}
	if len(cmd.Commands()) != 3 {
		t.Fatalf("expected 3 subcommands, got %d", len(cmd.Commands()))
	}
	names := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		names[sub.Name()] = true
	}
	for _, expected := range []string{"connect", "add", "info"} {
		if !names[expected] {
			t.Fatalf("expected %s subcommand", expected)
		}
	}
}

func TestDefaultDeps(t *testing.T) {
	d := defaultDeps()
	if d.GetToken == nil {
		t.Fatal("GetToken not set")
	}
	if d.HasRemote == nil {
		t.Fatal("HasRemote not set")
	}
	if d.GetRemoteURL == nil {
		t.Fatal("GetRemoteURL not set")
	}
	if d.DialWS == nil {
		t.Fatal("DialWS not set")
	}
	if d.Listen == nil {
		t.Fatal("Listen not set")
	}
	if d.RunPsql == nil {
		t.Fatal("RunPsql not set")
	}
}
