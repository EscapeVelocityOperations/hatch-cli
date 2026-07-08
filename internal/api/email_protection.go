package api

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// EmailProtection is an egg's email-allowlist protection state as the API
// returns it (h-oazj).
type EmailProtection struct {
	EmailProtected bool     `json:"email_protected"`
	Emails         []string `json:"emails"`
	Domains        []string `json:"domains"`
}

// SetEmailProtection enables email-allowlist protection, replacing both
// lists in one call (POST /v1/apps/{slug}/email-protect; D7 set-replace).
func (c *Client) SetEmailProtection(slug string, emails, domains []string) (*EmailProtection, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	body, err := json.Marshal(struct {
		Emails  []string `json:"emails"`
		Domains []string `json:"domains"`
	}{Emails: emails, Domains: domains})
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}
	resp, err := c.do("POST", "/apps/"+slug+"/email-protect", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var ep EmailProtection
	if err := json.NewDecoder(resp.Body).Decode(&ep); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &ep, nil
}

// GetEmailProtection returns the current email-allowlist protection state
// (GET /v1/apps/{slug}/email-protect).
func (c *Client) GetEmailProtection(slug string) (*EmailProtection, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	resp, err := c.do("GET", "/apps/"+slug+"/email-protect", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var ep EmailProtection
	if err := json.NewDecoder(resp.Body).Decode(&ep); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &ep, nil
}

// DeleteEmailProtection disables email-allowlist protection
// (DELETE /v1/apps/{slug}/email-protect).
func (c *Client) DeleteEmailProtection(slug string) error {
	if err := validateSlug(slug); err != nil {
		return err
	}
	resp, err := c.do("DELETE", "/apps/"+slug+"/email-protect", nil)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}
