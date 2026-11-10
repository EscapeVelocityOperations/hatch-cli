package invite

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/telemetry"
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
				ListPendingInvites: func(string) ([]api.PendingInvite, error) {
					return []api.PendingInvite{{ID: "inv-1", InviteToken: "tok-abc"}}, nil
				},
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
				ListPendingInvites: func(string) ([]api.PendingInvite, error) {
					return []api.PendingInvite{{ID: "inv-1", InviteToken: "tok-abc"}}, nil
				},
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

// TestAcceptDeclineResolveListID covers the list → accept/decline path with a
// row ID distinct from the invite token (h-e3s1k.2): the ID shown by
// 'hatch invite ls' resolves to the token sent to the API, the raw token
// still works, and an unknown ref is rejected without calling the API and
// without being echoed back.
func TestAcceptDeclineResolveListID(t *testing.T) {
	const (
		rowID  = "3f2b1c9e-0000-4000-8000-000000000001"
		secret = "0123456789abcdef0123456789abcdef-fictive"
	)
	pending := []api.PendingInvite{
		{ID: "other-row", InviteToken: "other-token"},
		{ID: rowID, InviteToken: secret, AppSlug: "their-app"},
	}

	type call struct{ accepted, declined []string }
	setup := func(c *call) {
		deps = &Deps{
			GetToken:           func() (string, error) { return "hatch_test", nil },
			ListPendingInvites: func(string) ([]api.PendingInvite, error) { return pending, nil },
			AcceptInvite: func(_, inviteToken string) (*api.Collaborator, error) {
				c.accepted = append(c.accepted, inviteToken)
				return &api.Collaborator{Status: "accepted"}, nil
			},
			DeclineInvite: func(_, inviteToken string) error {
				c.declined = append(c.declined, inviteToken)
				return nil
			},
		}
	}
	defer func() { deps = defaultDeps() }()

	for _, ref := range []string{rowID, secret} {
		var c call
		setup(&c)
		if err := runAccept(ref); err != nil {
			t.Fatalf("accept %q: %v", ref, err)
		}
		if err := runDecline(ref); err != nil {
			t.Fatalf("decline %q: %v", ref, err)
		}
		if len(c.accepted) != 1 || c.accepted[0] != secret {
			t.Errorf("accept %q sent %v, want [%s]", ref, c.accepted, secret)
		}
		if len(c.declined) != 1 || c.declined[0] != secret {
			t.Errorf("decline %q sent %v, want [%s]", ref, c.declined, secret)
		}
	}

	const unknown = "feedfacefeedfacefeedfacefeedface"
	var c call
	setup(&c)
	for name, run := range map[string]func(string) error{"accept": runAccept, "decline": runDecline} {
		err := run(unknown)
		if err == nil {
			t.Fatalf("%s unknown ref: expected error", name)
		}
		if strings.Contains(err.Error(), unknown) {
			t.Errorf("%s error echoes the ref: %v", name, err)
		}
		if !strings.Contains(err.Error(), "no pending invitation") {
			t.Errorf("%s error = %v, want no pending invitation", name, err)
		}
	}
	if len(c.accepted)+len(c.declined) != 0 {
		t.Errorf("unknown ref reached the API: %+v", c)
	}
}

// TestInviteArgsMarkedSensitive guards the telemetry opt-out on the commands
// that take an invite token positional (h-e3s1k.1).
func TestInviteArgsMarkedSensitive(t *testing.T) {
	for _, sub := range NewCmd().Commands() {
		name := sub.Name()
		got := sub.Annotations[telemetry.SensitiveArgsAnnotation] != ""
		want := name == "accept" || name == "decline"
		if got != want {
			t.Errorf("%s: sensitive-args annotation = %v, want %v", name, got, want)
		}
	}
}
