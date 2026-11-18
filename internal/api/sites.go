package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Hatch static sites (T-506, h-bgdsq.6): the client side of the site routes of
// hatch-api. Contract: hatch-api docs/plans/h-bgdsq-9-site-ingest-relay.md
// (manifest, then the missing blobs, then the confirmation).
//
//	POST /v1/apps/{slug}/site/ingest  application/json   manifest -> missing sha256
//	POST /v1/apps/{slug}/site/ingest  application/x-tar  tar of the missing blobs
//	POST /v1/apps/{slug}/site         application/json   records the version, makes it live
//
// Whether an egg is a site is read from GET /v1/apps/{slug}/deployments: a site
// publication is a row with "kind":"site" (T-505); image rows carry no kind.

// SiteFile is one entry of a site manifest. Path is relative to the site root,
// with forward slashes.
type SiteFile struct {
	Path   string `json:"path"`
	Sha256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// SiteIngestResult is the answer of the manifest step: the sha256 the
// platform does not hold yet, unique, in order of first appearance.
type SiteIngestResult struct {
	Missing      []string `json:"missing"`
	Files        int      `json:"files"`
	Unique       int      `json:"unique"`
	Bytes        int64    `json:"bytes"`
	MissingBytes int64    `json:"missing_bytes"`
}

// SiteBlobsResult is the answer of the blobs step.
type SiteBlobsResult struct {
	Created []struct {
		Sha256 string `json:"sha256"`
	} `json:"created"`
	Present      int   `json:"present"`
	BytesWritten int64 `json:"bytes_written"`
}

// SitePublishResult is the 201 answer of POST /v1/apps/{slug}/site.
type SitePublishResult struct {
	Version      int       `json:"version"`
	VersionID    string    `json:"version_id"`
	Files        int       `json:"files"`
	Bytes        int64     `json:"bytes"`
	CreatedAt    time.Time `json:"created_at"`
	URL          string    `json:"url"`
	DeploymentID string    `json:"deployment_id"`
}

// SiteError is a refusal of a site route: the HTTP status and the stable
// error code of the body ({"error":"<code>"}).
type SiteError struct {
	Status int
	Code   string
}

func (e *SiteError) Error() string {
	code := e.Code
	if code == "" {
		code = http.StatusText(e.Status)
	}
	msg := fmt.Sprintf("API error %d: %s", e.Status, code)
	if hint := siteErrorHints[e.Code]; hint != "" {
		msg += ": " + hint
	}
	return msg
}

// maxSiteAnswer bounds what is read of a 2xx answer of a site route. The
// largest one is the manifest answer: the sha256 the platform misses, 67
// bytes each, never more than the manifest entries, and the platform refuses
// a manifest over 10 MiB (413 site_manifest_too_large). 16 MiB covers it
// with room for the envelope; a platform default of 20000 files makes
// about 1.3 MiB. An answer over the bound is an error, never a silently cut
// body.
const maxSiteAnswer = 16 << 20

// maxSiteRefusal bounds what is read of a 4xx/5xx answer: a code and a
// message, nothing more.
const maxSiteRefusal = 1 << 20

// siteErrorHints say what each refusal of the contract means for the user
// (hatch-api h-bgdsq-9 "Refusals"; the 413 limits are the platform's
// HATCH_SITES_* settings).
var siteErrorHints = map[string]string{
	"sites_disabled":              "Hatch sites are not enabled on this platform",
	"sites_unavailable":           "the platform cannot store sites right now, try again later",
	"site_ingest_failed":          "the platform could not store the files, try again later",
	"site_unsupported_media_type": "the request was not understood (update hatch)",
	"invalid_site_manifest":       "the file list was refused (empty site, bad name or size)",
	"site_unsafe_path":            "a file name is not allowed (.., backslash or control character)",
	"site_duplicate_path":         "two files have the same name",
	"site_invalid_archive":        "the upload was refused, deploy again",
	"site_unsafe_entry":           "the upload was refused, deploy again",
	"site_invalid_blob_name":      "the upload was refused, deploy again",
	"site_duplicate_blob":         "the upload was refused, deploy again",
	"site_blob_hash_mismatch":     "a file changed while it was uploaded, deploy again",
	"site_trailing_data":          "the upload was refused, deploy again",
	"site_upload_interrupted":     "the upload was interrupted, deploy again",
	"site_manifest_too_large":     "the site has too many files to list in one deploy",
	"site_too_many_files":         "the site has more files than the platform allows",
	"site_file_too_large":         "a file is larger than the platform allows",
	"site_too_large":              "the site is larger than the platform allows",
	"site_quota_exceeded":         "the egg's site storage quota would be exceeded (all versions count): reduce the site",
}

// IsSitesDisabled reports whether err is the platform answering that sites
// are switched off (404 sites_disabled, before authentication).
func IsSitesDisabled(err error) bool {
	var se *SiteError
	return errors.As(err, &se) && se.Status == http.StatusNotFound && se.Code == "sites_disabled"
}

// HasSite reports whether the egg has published a Hatch site: its deployment
// history holds a "kind":"site" row (T-505). An API without sites never writes
// one. A 404 answers false: there is nothing to tell, and the artifact upload
// that follows reports an unknown egg itself.
func (c *Client) HasSite(slug string) (bool, error) {
	if err := validateSlug(slug); err != nil {
		return false, err
	}
	req, err := http.NewRequest(http.MethodGet, c.host+apiPath+"/apps/"+slug+"/deployments", nil)
	if err != nil {
		return false, redactURLError(err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, redactURLError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return false, fmt.Errorf("API error %d: %s", resp.StatusCode, RedactToken(strings.TrimSpace(string(msg))))
	}
	var rows []struct {
		Kind string `json:"kind"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return false, fmt.Errorf("decoding deployments: %w", err)
	}
	for _, r := range rows {
		if r.Kind == "site" {
			return true, nil
		}
	}
	return false, nil
}

// IngestSiteManifest posts the manifest (step 1) and returns the blobs the
// platform is missing. It writes no data.
func (c *Client) IngestSiteManifest(slug string, files []SiteFile) (*SiteIngestResult, error) {
	body, err := json.Marshal(struct {
		Files []SiteFile `json:"files"`
	}{files})
	if err != nil {
		return nil, fmt.Errorf("encoding manifest: %w", err)
	}
	var res SiteIngestResult
	if err := c.siteCall(slug, "/site/ingest", "application/json", bytes.NewReader(body), int64(len(body)), c.httpClient.Timeout, &res); err != nil {
		return nil, err
	}
	if res.Missing == nil {
		return nil, errors.New("decoding response: no missing list")
	}
	return &res, nil
}

// UploadSiteBlobs streams a gzipped tar of missing blobs (step 2): one entry
// per blob, named by its sha256. The body is read once, as it is sent, so its
// length is unknown and it goes chunked. Idempotent: the platform keeps what
// it already stored.
func (c *Client) UploadSiteBlobs(slug string, gzippedTar io.Reader) (*SiteBlobsResult, error) {
	var res SiteBlobsResult
	if err := c.siteCall(slug, "/site/ingest", "application/x-tar", gzippedTar, -1, artifactDeployTimeout, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// PublishSite records the manifest as the egg's next site version and makes
// it live (POST /v1/apps/{slug}/site). Each call is a new version, so it is
// retried only when the connection was never opened.
func (c *Client) PublishSite(slug string, files []SiteFile) (*SitePublishResult, error) {
	body, err := json.Marshal(struct {
		Files []SiteFile `json:"files"`
	}{files})
	if err != nil {
		return nil, fmt.Errorf("encoding manifest: %w", err)
	}
	var res SitePublishResult
	const maxAttempts = 3
	for attempt := 0; ; attempt++ {
		err = c.siteCall(slug, "/site", "application/json", bytes.NewReader(body), int64(len(body)), c.httpClient.Timeout, &res)
		if err == nil || !isDialError(err) || attempt+1 == maxAttempts {
			break
		}
		time.Sleep(time.Duration(1<<uint(attempt)) * pagesRetryDelay)
	}
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// siteCall posts body to /v1/apps/{slug}<path> and decodes a 2xx answer into
// out. A 4xx/5xx is a *SiteError carrying the body's error code. size < 0
// sends the body chunked.
func (c *Client) siteCall(slug, path, contentType string, body io.Reader, size int64, timeout time.Duration, out any) error {
	if err := validateSlug(slug); err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, c.host+apiPath+"/apps/"+slug+path, body)
	if err != nil {
		return redactURLError(err)
	}
	req.ContentLength = size
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", contentType)
	if contentType == "application/x-tar" {
		req.Header.Set("Content-Encoding", "gzip")
	}
	httpClient := *c.httpClient
	httpClient.Timeout = timeout
	resp, err := httpClient.Do(req)
	if err != nil {
		return redactURLError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	limit := int64(maxSiteAnswer)
	if resp.StatusCode >= 400 {
		limit = maxSiteRefusal
	}
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if resp.StatusCode >= 400 {
		if int64(len(data)) > limit {
			data = data[:limit]
		}
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) != nil || e.Error == "" {
			e.Error = RedactToken(strings.TrimSpace(string(data)))
			if len(e.Error) > 300 {
				e.Error = e.Error[:300] + "..."
			}
		}
		return &SiteError{Status: resp.StatusCode, Code: e.Error}
	}
	if int64(len(data)) > limit {
		return fmt.Errorf("the answer of the platform is over %d MiB: the site is too large to deploy in one go", limit>>20)
	}
	if readErr != nil {
		return fmt.Errorf("reading response: %w", readErr)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}
