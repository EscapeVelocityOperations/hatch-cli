package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// pathSegment validates and escapes a caller-supplied value (collaborator ID,
// invite token) placed into a single URL path segment. These reach the
// client from CLI args and, via the MCP tools, from LLM-supplied input, so
// they are untrusted: url.PathEscape encodes "/", "?", "#" and "%" so the
// value cannot add segments or a query, and the dot-segments "." / ".."
// (which PathEscape passes through unchanged) are rejected outright.
func pathSegment(kind, v string) (string, error) {
	switch v {
	case "":
		return "", fmt.Errorf("%s cannot be empty", kind)
	case ".", "..":
		return "", fmt.Errorf("invalid %s %q", kind, v)
	}
	return url.PathEscape(v), nil
}

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
	// json.Marshal, not %q: Go quoting is not JSON quoting (e.g. \x escapes).
	body, err := json.Marshal(struct {
		Email string `json:"email"`
	}{Email: email})
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}
	resp, err := c.do("POST", "/apps/"+slug+"/collaborators", bytes.NewReader(body))
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
	id, err := pathSegment("collaborator ID", collaboratorID)
	if err != nil {
		return err
	}
	resp, err := c.do("DELETE", "/apps/"+slug+"/collaborators/"+id, nil)
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
	t, err := pathSegment("invite token", token)
	if err != nil {
		return nil, err
	}
	resp, err := c.do("POST", "/invitations/"+t+"/accept", nil)
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
	t, err := pathSegment("invite token", token)
	if err != nil {
		return err
	}
	resp, err := c.do("POST", "/invitations/"+t+"/decline", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
