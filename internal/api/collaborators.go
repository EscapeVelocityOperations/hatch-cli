package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Collaborator represents an app_collaborators row, as returned by
// POST/GET /v1/apps/{slug}/collaborators and POST /v1/invitations/{token}/accept.
// AcceptedAt is deliberately not decoded here: the server currently returns it
// as sql.NullTime, which marshals as a nested {"Time":...,"Valid":...} object
// rather than a plain date; Status already conveys pending/accepted/declined.
type Collaborator struct {
	ID        string    `json:"id"`
	AppID     string    `json:"app_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// PendingInvite represents a pending invitation for the authenticated user,
// as returned by GET /v1/invitations/pending.
type PendingInvite struct {
	ID             string    `json:"id"`
	Email          string    `json:"email"`
	Role           string    `json:"role"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	AppSlug        string    `json:"app_slug"`
	AppName        string    `json:"app_name"`
	InvitedByEmail string    `json:"invited_by_email"`
}

// InviteCollaborator invites email to collaborate on an app (owner-only).
func (c *Client) InviteCollaborator(slug, email string) (*Collaborator, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	body := fmt.Sprintf(`{"email":%q}`, email)
	resp, err := c.do("POST", "/apps/"+slug+"/collaborators", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var collab Collaborator
	if err := json.NewDecoder(resp.Body).Decode(&collab); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &collab, nil
}

// ListCollaborators returns all collaborators for an app (owner-only).
func (c *Client) ListCollaborators(slug string) ([]Collaborator, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	resp, err := c.do("GET", "/apps/"+slug+"/collaborators", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var collabs []Collaborator
	if err := json.NewDecoder(resp.Body).Decode(&collabs); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return collabs, nil
}

// RemoveCollaborator removes a collaborator from an app by ID (owner-only).
func (c *Client) RemoveCollaborator(slug, collaboratorID string) error {
	if err := validateSlug(slug); err != nil {
		return err
	}
	resp, err := c.do("DELETE", "/apps/"+slug+"/collaborators/"+collaboratorID, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// ListPendingInvites returns pending invitations for the authenticated user.
func (c *Client) ListPendingInvites() ([]PendingInvite, error) {
	resp, err := c.do("GET", "/invitations/pending", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var invites []PendingInvite
	if err := json.NewDecoder(resp.Body).Decode(&invites); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return invites, nil
}

// AcceptInvite accepts a pending invitation by token.
func (c *Client) AcceptInvite(token string) (*Collaborator, error) {
	resp, err := c.do("POST", "/invitations/"+token+"/accept", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var collab Collaborator
	if err := json.NewDecoder(resp.Body).Decode(&collab); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &collab, nil
}

// DeclineInvite declines a pending invitation by token.
func (c *Client) DeclineInvite(token string) error {
	resp, err := c.do("POST", "/invitations/"+token+"/decline", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
