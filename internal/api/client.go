package api

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultHost = "https://api.gethatch.eu"
	apiPath     = "/v1"
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
	// Create transport with HTTP/2 disabled to avoid timeout issues
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
	}

	return &Client{
		host:  DefaultHost,
		token: token,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}
}

// do executes an HTTP request with Bearer auth and returns the response.
func (c *Client) do(method, path string, body io.Reader) (*http.Response, error) {
	url := c.host + apiPath + path
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
// logType can be "" for runtime logs or "build" for build logs.
func (c *Client) StreamLogs(slug string, tail int, follow bool, logType string, handler func(line string)) error {
	path := fmt.Sprintf("/apps/%s/logs?tail=%d&follow=%t", slug, tail, follow)
	if logType != "" {
		path += "&type=" + logType
	}

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

// AddAddon provisions an addon (e.g. "postgresql", "s3") for an app.
func (c *Client) AddAddon(slug, addonType string) (*Addon, error) {
	body := fmt.Sprintf(`{"type":%q}`, addonType)
	resp, err := c.do("POST", "/apps/"+slug+"/addons", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var addon Addon
	if err := json.NewDecoder(resp.Body).Decode(&addon); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &addon, nil
}

// ListDomains returns custom domains for an app.
func (c *Client) ListDomains(slug string) ([]Domain, error) {
	resp, err := c.do("GET", "/apps/"+slug+"/domains", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var domains []Domain
	if err := json.NewDecoder(resp.Body).Decode(&domains); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return domains, nil
}

// AddDomain configures a custom domain for an app.
func (c *Client) AddDomain(slug, domain string) (*Domain, error) {
	body := fmt.Sprintf(`{"domain":%q}`, domain)
	resp, err := c.do("POST", "/apps/"+slug+"/domains", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var d Domain
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &d, nil
}

// RemoveDomain removes a custom domain from an app.
func (c *Client) RemoveDomain(slug, domain string) error {
	resp, err := c.do("DELETE", "/apps/"+slug+"/domains/"+domain, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// GetLogs returns recent log lines (non-streaming).
func (c *Client) GetLogs(slug string, tail int, logType string) ([]string, error) {
	path := fmt.Sprintf("/apps/%s/logs?tail=%d&follow=false", slug, tail)
	if logType != "" {
		path += "&type=" + logType
	}
	resp, err := c.do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var lines []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			lines = append(lines, data)
		}
	}
	return lines, scanner.Err()
}
