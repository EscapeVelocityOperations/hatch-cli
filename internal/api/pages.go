package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Hatch pages (h-8iz8): pages published onto a protected egg and served by
// the Hatch gateway under /pages/<name>/ without a rebuild — the /v1
// endpoints of hatch-api.

// pageNameRegex is the API's page-name rule: one segment under /pages/.
var pageNameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,79}$`)

// NormalizePageName accepts "bilan", "/pages/bilan/" or "pages/bilan" and
// returns the bare page name.
func NormalizePageName(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "/")
	s = strings.TrimPrefix(s, "pages/")
	s = strings.Trim(s, "/")
	if !pageNameRegex.MatchString(s) {
		return "", fmt.Errorf("invalid page name %q: want lowercase letters, digits and hyphens (e.g. /pages/bilan-2026/)", raw)
	}
	return s, nil
}

// PagePublishMeta is the X-Pages-Metadata of a publication. AuthorLabel
// empty = the API uses the token's name; AuthorKind "" = agent.
type PagePublishMeta struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	AuthorKind  string `json:"author_kind,omitempty"`
	AuthorLabel string `json:"author_label,omitempty"`
	DisplayDate string `json:"display_date,omitempty"`
}

// PagePublishResult is the API's 201 answer.
type PagePublishResult struct {
	Name       string    `json:"name"`
	Title      string    `json:"title"`
	Version    int       `json:"version"`
	URL        string    `json:"url"`
	VersionURL string    `json:"version_url"`
	Files      int       `json:"files"`
	Bytes      int64     `json:"bytes"`
	DedupBytes int64     `json:"dedup_bytes"`
	CreatedAt  time.Time `json:"created_at"`
}

// PageSummary is one live page in GET /v1/apps/{slug}/pages.
type PageSummary struct {
	Name          string    `json:"name"`
	Title         string    `json:"title"`
	URL           string    `json:"url"`
	LatestVersion int       `json:"latest_version"`
	VersionURL    string    `json:"version_url"`
	Versions      int       `json:"versions"`
	AuthorLabel   *string   `json:"author_label"`
	DisplayDate   string    `json:"display_date"`
	Files         int       `json:"files"`
	Bytes         int64     `json:"bytes"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// PageList is GET /v1/apps/{slug}/pages. Gateway reports whether the egg is
// protected, i.e. whether its pages are actually served.
type PageList struct {
	Pages   []PageSummary `json:"pages"`
	Gateway bool          `json:"gateway"`
}

// PageVersion is one version in GET /v1/apps/{slug}/pages/{name}.
type PageVersion struct {
	Version     int       `json:"version"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Latest      bool      `json:"latest"`
	AuthorKind  string    `json:"author_kind"`
	AuthorLabel *string   `json:"author_label"`
	AuthorEmail string    `json:"author_email"`
	DisplayDate string    `json:"display_date"`
	Files       int       `json:"files"`
	Bytes       int64     `json:"bytes"`
	CreatedAt   time.Time `json:"created_at"`
}

// PageVersions is GET /v1/apps/{slug}/pages/{name}.
type PageVersions struct {
	Name          string        `json:"name"`
	Title         string        `json:"title"`
	URL           string        `json:"url"`
	LatestVersion int           `json:"latest_version"`
	NextVersion   int           `json:"next_version"`
	Versions      []PageVersion `json:"versions"`
}

// PageRemoveResult is DELETE /v1/apps/{slug}/pages/{name}.
type PageRemoveResult struct {
	Name           string `json:"name"`
	RemovedVersion int    `json:"removed_version,omitempty"`
	PageRemoved    bool   `json:"page_removed"`
	Purged         bool   `json:"purged,omitempty"`
}

// PublishPage uploads a page archive (tar.gz) as a new version of meta.Name.
// Retried on transport errors and 5xx like UploadArtifact: every attempt is
// a whole new upload and, until the API records it, nothing is published.
func (c *Client) PublishPage(slug string, archive []byte, meta PagePublishMeta) (*PagePublishResult, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	name, err := NormalizePageName(meta.Name)
	if err != nil {
		return nil, err
	}
	meta.Name = name
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("marshaling metadata: %w", err)
	}
	sum := sha256.Sum256(archive)

	uploadClient := *c.httpClient
	uploadClient.Timeout = artifactUploadTimeout
	uploadURL := c.host + apiPath + "/apps/" + slug + "/pages"

	const maxRetries = 3
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<uint(attempt-1)) * 2 * time.Second)
		}
		req, err := http.NewRequest(http.MethodPost, uploadURL, bytes.NewReader(archive))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Content-Type", "application/gzip")
		req.Header.Set("X-Pages-Metadata", string(metaJSON))
		req.Header.Set("X-Pages-Checksum", hex.EncodeToString(sum[:]))

		resp, err := uploadClient.Do(req)
		if err != nil {
			lastErr = err
			if isTimeoutError(err) {
				lastErr = fmt.Errorf("upload timed out after %s (attempt %d/%d): %w", artifactUploadTimeout, attempt+1, maxRetries, err)
			}
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("API error %d: %s", resp.StatusCode, RedactToken(strings.TrimSpace(string(body))))
			continue
		}
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, RedactToken(strings.TrimSpace(string(body))))
		}
		var res PagePublishResult
		if err := json.Unmarshal(body, &res); err != nil {
			return nil, fmt.Errorf("decoding response: %w", err)
		}
		return &res, nil
	}
	return nil, lastErr
}

// ListPages lists slug's live pages with their latest version.
func (c *Client) ListPages(slug string) (*PageList, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	resp, err := c.do(http.MethodGet, "/apps/"+slug+"/pages", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var l PageList
	if err := decodeJSON(resp.Body, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// ListPageVersions lists a page's versions, newest first.
func (c *Client) ListPageVersions(slug, page string) (*PageVersions, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	name, err := NormalizePageName(page)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(http.MethodGet, "/apps/"+slug+"/pages/"+name, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var v PageVersions
	if err := decodeJSON(resp.Body, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// RemovePage removes a page (all versions; its name keeps counting unless
// purge), or only version when version > 0.
func (c *Client) RemovePage(slug, page string, version int, purge bool) (*PageRemoveResult, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	name, err := NormalizePageName(page)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	if version > 0 {
		q.Set("version", strconv.Itoa(version))
	}
	if purge {
		q.Set("purge", "true")
	}
	path := "/apps/" + slug + "/pages/" + name
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	resp, err := c.do(http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var r PageRemoveResult
	if err := decodeJSON(resp.Body, &r); err != nil {
		return nil, err
	}
	return &r, nil
}
