package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultHost = "https://api.gethatch.eu"
	timeout     = 30 * time.Second
)

// Client is the Hatch API client.
type Client struct {
	host       string
	token      string
	httpClient *http.Client
}

// NewClient creates an API client with Bearer token auth.
func NewClient(token string) *Client {
	return &Client{
		host:  DefaultHost,
		token: token,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// do executes an HTTP request with Bearer auth and returns the response.
func (c *Client) do(method, path string, body io.Reader) (*http.Response, error) {
	url := c.host + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return resp, nil
}

// ListApps returns all apps for the authenticated user.
func (c *Client) ListApps() ([]App, error) {
	resp, err := c.do("GET", "/apps", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apps []App
	if err := json.NewDecoder(resp.Body).Decode(&apps); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return apps, nil
}

// GetApp returns details for a single app.
func (c *Client) GetApp(slug string) (*App, error) {
	resp, err := c.do("GET", "/apps/"+slug, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var app App
	if err := json.NewDecoder(resp.Body).Decode(&app); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &app, nil
}

// StreamLogs opens an SSE connection to stream app logs.
// It calls the handler for each log line. The caller should cancel via context or close.
func (c *Client) StreamLogs(slug string, tail int, follow bool, handler func(line string)) error {
	path := fmt.Sprintf("/apps/%s/logs?tail=%d&follow=%t", slug, tail, follow)

	req, err := http.NewRequest("GET", c.host+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "text/event-stream")

	// Use a client without timeout for streaming
	streamClient := &http.Client{}
	resp, err := streamClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		// SSE format: "data: <payload>"
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			handler(data)
		}
	}
	return scanner.Err()
}

// GetEnvVars returns environment variables for an app.
func (c *Client) GetEnvVars(slug string) ([]EnvVar, error) {
	resp, err := c.do("GET", "/apps/"+slug+"/env", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var vars []EnvVar
	if err := json.NewDecoder(resp.Body).Decode(&vars); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return vars, nil
}

// SetEnvVar sets an environment variable on an app.
func (c *Client) SetEnvVar(slug, key, value string) error {
	body := fmt.Sprintf(`{"key":%q,"value":%q}`, key, value)
	resp, err := c.do("PATCH", "/apps/"+slug+"/env", strings.NewReader(body))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// UnsetEnvVar removes an environment variable from an app.
func (c *Client) UnsetEnvVar(slug, key string) error {
	resp, err := c.do("DELETE", "/apps/"+slug+"/env/"+key, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// RestartApp restarts the specified app.
func (c *Client) RestartApp(slug string) error {
	resp, err := c.do("POST", "/apps/"+slug+"/restart", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// DeleteApp permanently deletes the specified app.
func (c *Client) DeleteApp(slug string) error {
	resp, err := c.do("DELETE", "/apps/"+slug, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
