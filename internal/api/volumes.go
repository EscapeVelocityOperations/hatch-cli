package api

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Volume mirrors the API's volume payload (GET /v1/apps/{slug}/volume).
type Volume struct {
	SizeMB    int    `json:"size_mb"`
	UsedMB    int    `json:"used_mb"`
	Status    string `json:"status"` // active | grace_deleting
	Mount     string `json:"mount"`
	OverQuota bool   `json:"over_quota"`
}

// EnableVolume provisions a persistent volume for slug, capped at the app's tier
// (POST /v1/apps/{slug}/volume). A size over the cap, or an already-enabled
// volume, is surfaced as the API error.
func (c *Client) EnableVolume(slug string, sizeMB int) error {
	if err := validateSlug(slug); err != nil {
		return err
	}
	body, err := json.Marshal(struct {
		SizeMB int `json:"size_mb"`
	}{SizeMB: sizeMB})
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}
	resp, err := c.do("POST", "/apps/"+slug+"/volume", bytes.NewReader(body))
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

// GetVolume returns the app's volume status (GET /v1/apps/{slug}/volume).
func (c *Client) GetVolume(slug string) (Volume, error) {
	if err := validateSlug(slug); err != nil {
		return Volume{}, err
	}
	resp, err := c.do("GET", "/apps/"+slug+"/volume", nil)
	if err != nil {
		return Volume{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	var v Volume
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return Volume{}, fmt.Errorf("decoding response: %w", err)
	}
	return v, nil
}

// DisableVolume detaches the app's volume (DELETE /v1/apps/{slug}/volume).
// now=true skips the grace period and deletes immediately (irreversible).
func (c *Client) DisableVolume(slug string, now bool) error {
	if err := validateSlug(slug); err != nil {
		return err
	}
	path := "/apps/" + slug + "/volume"
	if now {
		path += "?now=true"
	}
	resp, err := c.do("DELETE", path, nil)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}
