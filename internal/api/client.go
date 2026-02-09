package api

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	DefaultHost = "https://api.gethatch.eu"
	apiPath     = "/v1"
	timeout     = 30 * time.Second
)

var slugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// envKeyRegex validates environment variable key names.
// Allows standard POSIX env var names: start with letter or underscore,
// followed by alphanumeric or underscore, max 256 chars.
var envKeyRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,255}$`)

// domainRegex validates domain names. Allows standard domain characters
// (letters, digits, hyphens, dots) but rejects path separators, query
// strings, and other URL-manipulation characters.
var domainRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9.-]{0,253}[a-zA-Z0-9])?$`)

// validateSlug ensures slug values are safe for URL paths.
func validateSlug(slug string) error {
	if !slugRegex.MatchString(slug) {
		return fmt.Errorf("invalid slug %q: must be lowercase alphanumeric with hyphens", slug)
	}
	return nil
}

// ValidateSlug is the exported version of validateSlug for use by MCP handlers.
func ValidateSlug(slug string) error {
	return validateSlug(slug)
}

// ValidateEnvKey ensures environment variable key names are safe for URL paths.
// This is critical for UnsetEnvVar which places the key directly in the URL path.
func ValidateEnvKey(key string) error {
	if !envKeyRegex.MatchString(key) {
		return fmt.Errorf("invalid env key %q: must match [a-zA-Z_][a-zA-Z0-9_]*, max 256 chars", key)
	}
	return nil
}

// ValidateDomain ensures domain names are safe for URL paths.
// This is critical for RemoveDomain which places the domain directly in the URL path.
func ValidateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}
	if len(domain) > 255 {
		return fmt.Errorf("domain too long (max 255 characters)")
	}
	if !domainRegex.MatchString(domain) {
		return fmt.Errorf("invalid domain %q: must be a valid domain name (letters, digits, hyphens, dots only)", domain)
	}
	// Extra safety: reject any path-separator or URL-special characters
	if strings.ContainsAny(domain, "/?#@:") {
		return fmt.Errorf("invalid domain %q: contains URL-special characters", domain)
	}
	return nil
}

// RedactToken removes sensitive tokens from error messages.
func RedactToken(s string) string {
	re := regexp.MustCompile(`hatch_[a-zA-Z0-9_]+`)
	return re.ReplaceAllString(s, "hatch_****")
}

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

// NewTestClient creates an API client pointing to a custom host (for testing).
func NewTestClient(token, host string) *Client {
	c := NewClient(token)
	c.host = host
	return c
}

// do executes an HTTP request with Bearer auth and returns the response.
func (c *Client) do(method, path string, body io.Reader) (*http.Response, error) {
	url := c.host + apiPath + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, RedactToken(strings.TrimSpace(string(data))))
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
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
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

// CreateApp creates a new app with the given name.
// The server generates a unique slug (name + random suffix).
func (c *Client) CreateApp(name string) (*App, error) {
	body := fmt.Sprintf(`{"name":%q}`, name)
	resp, err := c.do("POST", "/apps", strings.NewReader(body))
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

// StreamLogs opens a WebSocket connection to stream app logs.
// It calls the handler for each log line. The caller should cancel via context or close.
// logType can be "" for runtime logs or "build" for build logs.
func (c *Client) StreamLogs(slug string, tail int, follow bool, logType string, handler func(line string)) error {
	if err := validateSlug(slug); err != nil {
		return err
	}
	// Build WebSocket URL from HTTP host
	wsURL, err := url.Parse(c.host)
	if err != nil {
		return fmt.Errorf("parsing host: %w", err)
	}
	if wsURL.Scheme == "https" {
		wsURL.Scheme = "wss"
	} else {
		wsURL.Scheme = "ws"
	}
	wsURL.Path = apiPath + fmt.Sprintf("/apps/%s/logs", slug)

	query := url.Values{}
	query.Set("lines", fmt.Sprintf("%d", tail))
	query.Set("follow", fmt.Sprintf("%t", follow))
	if logType != "" {
		query.Set("type", logType)
	}
	wsURL.RawQuery = query.Encode()

	// Connect with auth header
	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.token)

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL.String(), header)
	if err != nil {
		if resp != nil && resp.StatusCode >= 400 {
			data, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("API error %d: %s", resp.StatusCode, RedactToken(strings.TrimSpace(string(data))))
		}
		return fmt.Errorf("connecting to log stream: %w", err)
	}
	defer conn.Close()

	// Read messages until connection closes
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return nil
			}
			return err
		}
		handler(string(message))
	}
}

// GetEnvVars returns environment variables for an app.
func (c *Client) GetEnvVars(slug string) ([]EnvVar, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
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
	if err := validateSlug(slug); err != nil {
		return err
	}
	body := fmt.Sprintf(`{"key":%q,"value":%q}`, key, value)
	resp, err := c.do("POST", "/apps/"+slug+"/env", strings.NewReader(body))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// UnsetEnvVar removes an environment variable from an app.
func (c *Client) UnsetEnvVar(slug, key string) error {
	if err := validateSlug(slug); err != nil {
		return err
	}
	if err := ValidateEnvKey(key); err != nil {
		return err
	}
	resp, err := c.do("DELETE", "/apps/"+slug+"/env/"+key, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// RestartApp restarts the specified app.
func (c *Client) RestartApp(slug string) error {
	if err := validateSlug(slug); err != nil {
		return err
	}
	resp, err := c.do("POST", "/apps/"+slug+"/restart", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// DeleteApp permanently deletes the specified app.
func (c *Client) DeleteApp(slug string) error {
	if err := validateSlug(slug); err != nil {
		return err
	}
	resp, err := c.do("DELETE", "/apps/"+slug, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// AddAddon provisions an addon (e.g. "postgresql", "s3") for an app.
func (c *Client) AddAddon(slug, addonType string) (*Addon, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
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

// ListAddons returns addons for an app (includes usage stats for postgresql).
func (c *Client) ListAddons(slug string) ([]Addon, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	resp, err := c.do("GET", "/apps/"+slug+"/addons", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var addons []Addon
	if err := json.NewDecoder(resp.Body).Decode(&addons); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return addons, nil
}

// ListDomains returns custom domains for an app.
func (c *Client) ListDomains(slug string) ([]Domain, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
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
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
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
	if err := validateSlug(slug); err != nil {
		return err
	}
	if err := ValidateDomain(domain); err != nil {
		return err
	}
	resp, err := c.do("DELETE", "/apps/"+slug+"/domains/"+domain, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// GetLogs returns recent log lines (non-streaming).
func (c *Client) GetLogs(slug string, tail int, logType string) ([]string, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
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

// UploadArtifact uploads a pre-built tar.gz artifact for deployment.
func (c *Client) UploadArtifact(slug string, artifact io.Reader, runtime, startCommand string) error {
	if err := validateSlug(slug); err != nil {
		return err
	}
	url := c.host + apiPath + "/apps/" + slug + "/artifact"
	req, err := http.NewRequest("POST", url, artifact)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/gzip")

	// hatch-api expects metadata as a single JSON header
	metadata := struct {
		Runtime      string `json:"runtime"`
		StartCommand string `json:"startCommand"`
	}{
		Runtime:      runtime,
		StartCommand: startCommand,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}
	req.Header.Set("X-Artifact-Metadata", string(metadataJSON))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed (%d): %s", resp.StatusCode, RedactToken(strings.TrimSpace(string(data))))
	}
	return nil
}

// ListKeys returns API keys for the authenticated user.
func (c *Client) ListKeys() ([]APIKey, error) {
	resp, err := c.do("GET", "/keys", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var keys []APIKey
	if err := json.NewDecoder(resp.Body).Decode(&keys); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return keys, nil
}

// GetAppStatus returns the raw JSON status response for an app.
// This includes app info, last deployment status, and custom domains.
func (c *Client) GetAppStatus(slug string) (json.RawMessage, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	resp, err := c.do("GET", "/apps/"+slug+"/status", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	return json.RawMessage(data), nil
}

// EnergyStatus represents energy information for the user's account.
type EnergyStatus struct {
	Tier               string   `json:"tier"`
	DailyRemaining     int      `json:"daily_remaining_minutes"`
	DailyLimit         int      `json:"daily_limit_minutes"`
	WeeklyRemaining    int      `json:"weekly_remaining_minutes"`
	WeeklyLimit        int      `json:"weekly_limit_minutes"`
	ResetsAt           string   `json:"resets_at"`
	EggsActive         int      `json:"eggs_active"`
	EggsSleeping       int      `json:"eggs_sleeping"`
	EggsLimit          int      `json:"eggs_limit"`
	AlwaysOnEggs       []string `json:"always_on_eggs"`
	BoostedEggs        []string `json:"boosted_eggs,omitempty"`
}

// AppEnergy represents energy information for a specific app.
type AppEnergy struct {
	Slug               string  `json:"slug"`
	Status             string  `json:"status"`
	Plan               string  `json:"plan"`
	AlwaysOn           bool    `json:"always_on"`
	Boosted            bool    `json:"boosted"`
	BoostExpiresAt     *string `json:"boost_expires_at,omitempty"`
	DailyRemainingMin  int     `json:"daily_remaining_minutes"`
	DailyLimitMin      int     `json:"daily_limit_minutes"`
	DailyUsedMin       int     `json:"daily_used_minutes"`
	WeeklyRemainingMin int     `json:"weekly_remaining_minutes"`
	WeeklyLimitMin     int     `json:"weekly_limit_minutes"`
	WeeklyUsedMin      int     `json:"weekly_used_minutes"`
	DailyResetsAt      string  `json:"daily_resets_at"`
	WeeklyResetsAt     string  `json:"weekly_resets_at"`
	BonusEnergy        int     `json:"bonus_energy"`
}

// BoostCheckoutResponse represents a boost checkout session.
type BoostCheckoutResponse struct {
	CheckoutURL string `json:"checkout_url"`
	Duration    string `json:"duration"`
	AmountEur   string `json:"amount_eur"`
}

// GetAccountEnergy returns the user's account energy status.
func (c *Client) GetAccountEnergy() (*EnergyStatus, error) {
	resp, err := c.do("GET", "/account/energy", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var energy EnergyStatus
	if err := json.NewDecoder(resp.Body).Decode(&energy); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &energy, nil
}

// GetAppEnergy returns energy status for a specific app.
func (c *Client) GetAppEnergy(slug string) (*AppEnergy, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	resp, err := c.do("GET", "/apps/"+slug+"/energy", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var energy AppEnergy
	if err := json.NewDecoder(resp.Body).Decode(&energy); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &energy, nil
}

// BoostCheckout creates a Stripe checkout session for boost purchase.
// Returns a checkout URL to open in browser.
func (c *Client) BoostCheckout(slug, duration string) (*BoostCheckoutResponse, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	body := fmt.Sprintf(`{"egg_slug":%q,"duration":%q}`, slug, duration)
	resp, err := c.do("POST", "/billing/boost-checkout", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result BoostCheckoutResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}
