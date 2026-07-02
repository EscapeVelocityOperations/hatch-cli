package invite

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
)

func TestRunList(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		invites     []api.PendingInvite
		listErr     error
		wantErr     bool
		errContains string
	}{
		{name: "no token", token: "", wantErr: true, errContains: "not logged in"},
		{
			name:  "success with invites",
			token: "test-token",
			invites: []api.PendingInvite{
				{ID: "c1", AppSlug: "their-app", AppName: "Their App", InvitedByEmail: "owner@example.com", CreatedAt: time.Now()},
			},
			wantErr: false,
		},
		{name: "success with none", token: "test-token", invites: []api.PendingInvite{}, wantErr: false},
		{name: "API error", token: "test-token", listErr: errors.New("boom"), wantErr: true, errContains: "fetching invitations"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps = &Deps{
				GetToken: func() (string, error) { return tt.token, nil },
				ListPendingInvites: func(token string) ([]api.PendingInvite, error) {
					return tt.invites, tt.listErr
				},
			}
			defer func() { deps = defaultDeps() }()

			err := runList()
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

func TestRunAccept(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		collab      *api.Collaborator
		acceptErr   error
		wantErr     bool
		errContains string
	}{
		{name: "no token", token: "", wantErr: true, errContains: "not logged in"},
		{
			name:    "success",
			token:   "test-token",
			collab:  &api.Collaborator{ID: "c1", AppID: "a1", Status: "accepted"},
			wantErr: false,
		},
		{
			name:        "wrong email",
			token:       "test-token",
			acceptErr:   errors.New("API error 403: this invitation was sent to a different email address"),
			wantErr:     true,
			errContains: "accepting invitation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps = &Deps{
				GetToken: func() (string, error) { return tt.token, nil },
				AcceptInvite: func(token, inviteToken string) (*api.Collaborator, error) {
					return tt.collab, tt.acceptErr
				},
			}
			defer func() { deps = defaultDeps() }()

			err := runAccept("tok-abc")
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

func TestRunDecline(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		declineErr  error
		wantErr     bool
		errContains string
	}{
		{name: "no token", token: "", wantErr: true, errContains: "not logged in"},
		{name: "success", token: "test-token", wantErr: false},
		{name: "API error", token: "test-token", declineErr: errors.New("boom"), wantErr: true, errContains: "declining invitation"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps = &Deps{
				GetToken: func() (string, error) { return tt.token, nil },
				DeclineInvite: func(token, inviteToken string) error {
					return tt.declineErr
				},
			}
			defer func() { deps = defaultDeps() }()

			err := runDecline("tok-abc")
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
