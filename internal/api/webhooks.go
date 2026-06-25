package api

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Webhook is an outbound deploy webhook as the API returns it. Secret is set
// only by CreateWebhook — the one time the plaintext signing secret is shown.
type Webhook struct {
	ID     string   `json:"id"`
	URL    string   `json:"url"`
	Events []string `json:"events"`
	Secret string   `json:"secret,omitempty"`
}

// CreateWebhook registers a webhook for slug (POST /v1/apps/{slug}/webhooks).
// The returned Webhook.Secret is the plaintext signing secret, available only
// from this call.
func (c *Client) CreateWebhook(slug, url string, events []string) (*Webhook, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	body, err := json.Marshal(struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
	}{URL: url, Events: events})
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}
	resp, err := c.do("POST", "/apps/"+slug+"/webhooks", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var wh Webhook
	if err := json.NewDecoder(resp.Body).Decode(&wh); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &wh, nil
}

// ListWebhooks returns the app's webhooks (GET /v1/apps/{slug}/webhooks);
// secrets are never returned here.
func (c *Client) ListWebhooks(slug string) ([]Webhook, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	resp, err := c.do("GET", "/apps/"+slug+"/webhooks", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var whs []Webhook
	if err := json.NewDecoder(resp.Body).Decode(&whs); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return whs, nil
}

// DeleteWebhook removes a webhook (DELETE /v1/apps/{slug}/webhooks/{id}).
func (c *Client) DeleteWebhook(slug, id string) error {
	if err := validateSlug(slug); err != nil {
		return err
	}
	resp, err := c.do("DELETE", "/apps/"+slug+"/webhooks/"+id, nil)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

// TestWebhook triggers a server-side signed `ping` delivery to the webhook
// (POST /v1/apps/{slug}/webhooks/{id}/test).
func (c *Client) TestWebhook(slug, id string) error {
	if err := validateSlug(slug); err != nil {
		return err
	}
	resp, err := c.do("POST", "/apps/"+slug+"/webhooks/"+id+"/test", nil)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}
