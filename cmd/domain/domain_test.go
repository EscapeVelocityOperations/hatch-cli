package domain

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
)

func TestListDomains(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		tokenErr    error
		domains     []api.Domain
		listErr     error
		wantErr     bool
		errContains string
	}{
		{
			name:        "no token",
			token:       "",
			wantErr:     true,
			errContains: "not logged in",
		},
		{
			name:     "success with domains",
			token:    "test-token",
			domains:  []api.Domain{{Domain: "example.com", Status: "active"}},
			wantErr:  false,
		},
		{
			name:    "success with no domains",
			token:   "test-token",
			domains: []api.Domain{},
			wantErr: false,
		},
		{
			name:        "API error",
			token:       "test-token",
			listErr:     errors.New("API error"),
			wantErr:     true,
			errContains: "fetching domains",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps = &Deps{
				GetToken: func() (string, error) { return tt.token, tt.tokenErr },
				ListDomains: func(token, slug string) ([]api.Domain, error) {
					return tt.domains, tt.listErr
				},
			}
			defer func() { deps = defaultDeps() }()

			err := runList("test-app")
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errContains != "" && !bytes.Contains([]byte(err.Error()), []byte(tt.errContains)) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestAddDomain(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		domain      *api.Domain
		addErr      error
		wantErr     bool
		errContains string
	}{
		{
			name:        "no token",
			token:       "",
			wantErr:     true,
			errContains: "not logged in",
		},
		{
			name:    "success",
			token:   "test-token",
			domain:  &api.Domain{Domain: "example.com", Status: "pending", CNAME: "example.app.gethatch.eu"},
			wantErr: false,
		},
		{
			name:        "API error",
			token:       "test-token",
			addErr:      errors.New("domain already exists"),
			wantErr:     true,
			errContains: "adding domain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps = &Deps{
				GetToken: func() (string, error) { return tt.token, nil },
				AddDomain: func(token, slug, domain string) (*api.Domain, error) {
					return tt.domain, tt.addErr
				},
			}
			defer func() { deps = defaultDeps() }()

			// Capture output
			old := os.Stdout
			os.Stdout, _ = os.Open(os.DevNull)
			defer func() { os.Stdout = old }()

			err := runAdd("test-app", "example.com")
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errContains != "" && !bytes.Contains([]byte(err.Error()), []byte(tt.errContains)) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestRemoveDomain(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		removeErr   error
		wantErr     bool
		errContains string
	}{
		{
			name:        "no token",
			token:       "",
			wantErr:     true,
			errContains: "not logged in",
		},
		{
			name:    "success",
			token:   "test-token",
			wantErr: false,
		},
		{
			name:        "API error",
			token:       "test-token",
			removeErr:   errors.New("domain not found"),
			wantErr:     true,
			errContains: "removing domain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps = &Deps{
				GetToken: func() (string, error) { return tt.token, nil },
				RemoveDomain: func(token, slug, domain string) error {
					return tt.removeErr
				},
			}
			defer func() { deps = defaultDeps() }()

			err := runRemove("test-app", "example.com")
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errContains != "" && !bytes.Contains([]byte(err.Error()), []byte(tt.errContains)) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestNewCmd(t *testing.T) {
	cmd := NewCmd()

	if cmd.Use != "domain" {
		t.Errorf("expected Use 'domain', got %q", cmd.Use)
	}

	// Check subcommands exist
	subCmds := cmd.Commands()
	names := make(map[string]bool)
	for _, sub := range subCmds {
		names[sub.Use] = true
	}

	for _, want := range []string{"list", "add <domain>", "remove <domain>"} {
		if !names[want] {
			t.Errorf("missing subcommand %q", want)
		}
	}
}
