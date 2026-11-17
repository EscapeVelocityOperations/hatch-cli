package api

import (
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHasSite(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
		err    bool
	}{
		{"site row", 200, `[{"id":"b","kind":"site","site_version":2},{"id":"a","status":"live"}]`, true, false},
		{"image rows only (old API)", 200, `[{"id":"a","status":"live","image_url":"x"}]`, false, false},
		{"empty history", 200, `[]`, false, false},
		{"404 tells nothing", 404, `{"error":"app not found"}`, false, false},
		{"500 is an error", 500, `{"error":"internal server error"}`, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/apps/egg-1/deployments" || r.Header.Get("Authorization") != "Bearer tok" {
					t.Errorf("request %s %s auth %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			got, err := NewTestClient("tok", srv.URL).HasSite("egg-1")
			if (err != nil) != tc.err || got != tc.want {
				t.Fatalf("HasSite = %v, %v; want %v, err %v", got, err, tc.want, tc.err)
			}
		})
	}
}

// The wire of the three site calls: route, types, gzip encoding, chunked
// tar, and the refusal code kept as a *SiteError.
func TestSiteCalls_Wire(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen = append(seen, r.Method+" "+r.URL.Path+" "+r.Header.Get("Content-Type")+" "+r.Header.Get("Content-Encoding"))
		switch {
		case r.URL.Path == "/v1/apps/egg-1/site/ingest" && r.Header.Get("Content-Type") == "application/json":
			if string(body) != `{"files":[{"path":"index.html","sha256":"`+strings.Repeat("a", 64)+`","size":3}]}` {
				t.Errorf("manifest body %s", body)
			}
			_, _ = io.WriteString(w, `{"missing":["`+strings.Repeat("a", 64)+`"],"files":1,"unique":1,"bytes":3,"missing_bytes":3}`)
		case r.URL.Path == "/v1/apps/egg-1/site/ingest":
			if r.ContentLength != -1 || len(r.TransferEncoding) == 0 {
				t.Errorf("tar not chunked: length %d, TE %v", r.ContentLength, r.TransferEncoding)
			}
			_, _ = io.WriteString(w, `{"created":[{"sha256":"x","mtime":"t"}],"present":0,"bytes_written":3}`)
		case r.URL.Path == "/v1/apps/egg-1/site":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"version":4,"version_id":"v","files":1,"bytes":3,"url":"https://egg-1.nest/","deployment_id":"d"}`)
		}
	}))
	defer srv.Close()
	c := NewTestClient("tok", srv.URL)
	files := []SiteFile{{Path: "index.html", Sha256: strings.Repeat("a", 64), Size: 3}}

	ing, err := c.IngestSiteManifest("egg-1", files)
	if err != nil || len(ing.Missing) != 1 || ing.MissingBytes != 3 {
		t.Fatalf("ingest = %+v, %v", ing, err)
	}
	pr, pw := io.Pipe()
	go func() {
		gz := gzip.NewWriter(pw)
		_, _ = gz.Write([]byte("tar"))
		_ = gz.Close()
		_ = pw.Close()
	}()
	up, err := c.UploadSiteBlobs("egg-1", pr)
	if err != nil || len(up.Created) != 1 || up.BytesWritten != 3 {
		t.Fatalf("upload = %+v, %v", up, err)
	}
	pub, err := c.PublishSite("egg-1", files)
	if err != nil || pub.Version != 4 || pub.DeploymentID != "d" {
		t.Fatalf("publish = %+v, %v", pub, err)
	}
	want := []string{
		"POST /v1/apps/egg-1/site/ingest application/json ",
		"POST /v1/apps/egg-1/site/ingest application/x-tar gzip",
		"POST /v1/apps/egg-1/site application/json ",
	}
	if strings.Join(seen, "|") != strings.Join(want, "|") {
		t.Fatalf("requests:\n%s\nwant:\n%s", strings.Join(seen, "\n"), strings.Join(want, "\n"))
	}
}

func TestSiteError_CodeAndHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/apps/quota/site/ingest":
			w.WriteHeader(413)
			_, _ = io.WriteString(w, `{"error":"site_quota_exceeded"}`)
		case "/v1/apps/off/site/ingest":
			w.WriteHeader(404)
			_, _ = io.WriteString(w, `{"error":"sites_disabled","message":"Hatch sites are not available on this platform yet"}`)
		default:
			w.WriteHeader(502)
			_, _ = io.WriteString(w, "<html>bad gateway</html>")
		}
	}))
	defer srv.Close()
	c := NewTestClient("tok", srv.URL)
	files := []SiteFile{{Path: "a", Sha256: strings.Repeat("a", 64), Size: 1}}

	_, err := c.IngestSiteManifest("quota", files)
	var se *SiteError
	if !errors.As(err, &se) || se.Status != 413 || se.Code != "site_quota_exceeded" {
		t.Fatalf("err = %#v", err)
	}
	if err.Error() != "API error 413: site_quota_exceeded: the egg's site storage quota would be exceeded (all versions count): reduce the site" {
		t.Fatalf("message = %q", err)
	}
	if IsSitesDisabled(err) {
		t.Fatal("a 413 is not sites_disabled")
	}

	_, err = c.IngestSiteManifest("off", files)
	if !IsSitesDisabled(err) {
		t.Fatalf("err = %v, want sites_disabled", err)
	}

	_, err = c.IngestSiteManifest("proxy", files)
	if !errors.As(err, &se) || se.Status != 502 || se.Code != "<html>bad gateway</html>" {
		t.Fatalf("err = %#v", err)
	}
}

// A publication is a new version each time: a 5xx is not retried; a
// connection never opened is.
func TestPublishSite_RetriesOnlyUnopenedConnections(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(503)
	}))
	defer srv.Close()
	files := []SiteFile{{Path: "a", Sha256: strings.Repeat("a", 64), Size: 1}}
	if _, err := NewTestClient("tok", srv.URL).PublishSite("egg-1", files); err == nil || calls != 1 {
		t.Fatalf("err = %v after %d calls, want one call", err, calls)
	}

	pagesRetryDelay = time.Millisecond
	t.Cleanup(func() { pagesRetryDelay = 2 * time.Second })
	closed := httptest.NewServer(http.NotFoundHandler())
	url := closed.URL
	closed.Close()
	_, err := NewTestClient("tok", url).PublishSite("egg-1", files)
	if err == nil || !isDialError(err) {
		t.Fatalf("err = %v, want a dial error after retries", err)
	}
}
