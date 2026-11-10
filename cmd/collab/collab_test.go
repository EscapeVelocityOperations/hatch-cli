package collab

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
)

func TestRunAdd(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		collab      *api.Collaborator
		inviteErr   error
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
			name:  "success",
			token: "test-token",
			collab: &api.Collaborator{
				ID: "c1", Email: "friend@example.com", Status: "pending", CreatedAt: time.Now(),
			},
			wantErr: false,
		},
		{
			name:        "max reached",
			token:       "test-token",
			inviteErr:   errors.New("API error 409: maximum collaborators per egg reached"),
			wantErr:     true,
			errContains: "inviting collaborator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps = &Deps{
				GetToken: func() (string, error) { return tt.token, nil },
				InviteCollaborator: func(token, slug, email string) (*api.Collaborator, error) {
					return tt.collab, tt.inviteErr
				},
			}
			defer func() { deps = defaultDeps() }()

			err := runAdd("my-app", "friend@example.com")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				} else if tt.errContains != "" && !bytes.Contains([]byte(err.Error()), []byte(tt.errContains)) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestRunList(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		collabs     []api.Collaborator
		listErr     error
		wantErr     bool
		errContains string
	}{
		{name: "no token", token: "", wantErr: true, errContains: "not logged in"},
		{
			name:    "success with collaborators",
			token:   "test-token",
			collabs: []api.Collaborator{{ID: "c1", Email: "friend@example.com", Status: "accepted"}},
			wantErr: false,
		},
		{name: "success with none", token: "test-token", collabs: []api.Collaborator{}, wantErr: false},
		{name: "API error", token: "test-token", listErr: errors.New("boom"), wantErr: true, errContains: "fetching collaborators"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps = &Deps{
				GetToken: func() (string, error) { return tt.token, nil },
				ListCollaborators: func(token, slug string) ([]api.Collaborator, error) {
					return tt.collabs, tt.listErr
				},
			}
			defer func() { deps = defaultDeps() }()

			err := runList("my-app")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				} else if tt.errContains != "" && !bytes.Contains([]byte(err.Error()), []byte(tt.errContains)) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestRunRemove(t *testing.T) {
	tests := []struct {
		name         string
		token        string
		target       string
		collabs      []api.Collaborator
		listErr      error
		removeErr    error
		wantErr      bool
		errContains  string
		wantRemoveID string
	}{
		{name: "no token", token: "", target: "friend@example.com", wantErr: true, errContains: "not logged in"},
		{
			name:         "remove by raw id",
			token:        "test-token",
			target:       "c1",
			collabs:      []api.Collaborator{{ID: "c1", Email: "friend@example.com"}},
			wantRemoveID: "c1",
		},
		{
			name:         "remove by email",
			token:        "test-token",
			target:       "friend@example.com",
			collabs:      []api.Collaborator{{ID: "c1", Email: "friend@example.com"}, {ID: "c2", Email: "other@example.com"}},
			wantRemoveID: "c1",
		},
		{
			name:        "email not found",
			token:       "test-token",
			target:      "nobody@example.com",
			collabs:     []api.Collaborator{{ID: "c1", Email: "friend@example.com"}},
			wantErr:     true,
			errContains: "no collaborator found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotRemoveID string
			deps = &Deps{
				GetToken: func() (string, error) { return tt.token, nil },
				ListCollaborators: func(token, slug string) ([]api.Collaborator, error) {
					return tt.collabs, tt.listErr
				},
				RemoveCollaborator: func(token, slug, collaboratorID string) error {
					gotRemoveID = collaboratorID
					return tt.removeErr
				},
			}
			defer func() { deps = defaultDeps() }()

			err := runRemove("my-app", tt.target)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				} else if tt.errContains != "" && !bytes.Contains([]byte(err.Error()), []byte(tt.errContains)) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotRemoveID != tt.wantRemoveID {
				t.Errorf("removed id = %q, want %q", gotRemoveID, tt.wantRemoveID)
			}
		})
	}
}
