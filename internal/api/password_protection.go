package api

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// PasswordProtection is an egg's password-protection state as the API
// returns it (h-macc / h-abmr).
type PasswordProtection struct {
	Protected bool `json:"protected"`
}

// SetPasswordProtection enables password protection with the given password
// (POST /v1/apps/{slug}/protect).
func (c *Client) SetPasswordProtection(slug, password string) (*PasswordProtection, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	body, err := json.Marshal(struct {
		Password string `json:"password"`
	}{Password: password})
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}
	resp, err := c.do("POST", "/apps/"+slug+"/protect", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var pp PasswordProtection
	if err := json.NewDecoder(resp.Body).Decode(&pp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &pp, nil
}
