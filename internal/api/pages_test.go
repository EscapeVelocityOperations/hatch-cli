package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNormalizePageName(t *testing.T) {
	for in, want := range map[string]string{"bilan": "bilan", "/pages/bilan/": "bilan", "pages/bilan-2": "bilan-2"} {
		if got, err := NormalizePageName(in); err != nil || got != want {
			t.Errorf("NormalizePageName(%q) = %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "/pages/", "Bilan", "a/b", "../x", "bilan?x=1"} {
		if _, err := NormalizePageName(in); err == nil {
			t.Errorf("NormalizePageName(%q) accepted", in)
		}
	}
}

func TestPublishPage_SendsArchiveMetadataAndChecksum(t *testing.T) {
	archive := []byte("tgz-bytes")
	var got struct {
		method, path, ctype, auth, meta, sum string
		body                                 []byte
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path, got.ctype = r.Method, r.URL.Path, r.Header.Get("Content-Type")
		got.auth, got.meta, got.sum = r.Header.Get("Authorization"), r.Header.Get("X-Pages-Metadata"), r.Header.Get("X-Pages-Checksum")
		got.body, _ = io.ReadAll(r.Body)
		w.WriteHeader(201)
		_, _ = io.WriteString(w, `{"name":"bilan","version":2,"url":"https://e.hatchapp.eu/pages/bilan/","version_url":"https://e.hatchapp.eu/pages/bilan/v2/","files":3,"bytes":10,"dedup_bytes":4}`)
	}))
	defer srv.Close()
	res, err := NewTestClient("tok", srv.URL).PublishPage("my-egg", archive, PagePublishMeta{Name: "/pages/bilan/", Title: "Bilan", AuthorLabel: "Claude (bosco)"})
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(archive)
	if got.method != "POST" || got.path != "/v1/apps/my-egg/pages" || got.ctype != "application/gzip" || got.auth != "Bearer tok" ||
		got.sum != hex.EncodeToString(sum[:]) || !bytes.Equal(got.body, archive) {
		t.Fatalf("request = %+v", got)
	}
	var meta map[string]string
	_ = json.Unmarshal([]byte(got.meta), &meta)
	if meta["name"] != "bilan" || meta["title"] != "Bilan" || meta["author_label"] != "Claude (bosco)" {
		t.Fatalf("metadata = %s", got.meta)
	}
	if _, ok := meta["display_date"]; ok {
		t.Fatalf("empty fields must be omitted: %s", got.meta)
	}
	if res.Version != 2 || res.VersionURL != "https://e.hatchapp.eu/pages/bilan/v2/" || res.DedupBytes != 4 {
		t.Fatalf("result = %+v", res)
	}
}

func TestPublishPage_RetriesOnlyWhatCannotHavePublished(t *testing.T) {
	pagesRetryDelay = time.Millisecond
	t.Cleanup(func() { pagesRetryDelay = 2 * time.Second })
	var calls, status int32
	status = 503
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(int(atomic.LoadInt32(&status)))
			return
		}
		w.WriteHeader(201)
		_, _ = io.WriteString(w, `{"name":"x","version":1}`)
	}))
	defer srv.Close()
	if _, err := NewTestClient("tok", srv.URL).PublishPage("e", []byte("a"), PagePublishMeta{Name: "x"}); err != nil || atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("503 retry: err=%v calls=%d", err, calls)
	}

	// N1: a 502 (or a lost answer) may follow the commit: no second vN+1.
	atomic.StoreInt32(&calls, 0)
	atomic.StoreInt32(&status, 502)
	_, err := NewTestClient("tok", srv.URL).PublishPage("e", []byte("a"), PagePublishMeta{Name: "x"})
	if !errors.Is(err, errPublishUnknown) || atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("502: err=%v calls=%d", err, calls)
	}
	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = io.ReadAll(r.Body)
		hj, _ := w.(http.Hijacker)
		conn, _, _ := hj.Hijack()
		_ = conn.Close() // answer lost after the body was read
	}))
	defer hang.Close()
	atomic.StoreInt32(&calls, 0)
	_, err = NewTestClient("tok", hang.URL).PublishPage("e", []byte("a"), PagePublishMeta{Name: "x"})
	if !errors.Is(err, errPublishUnknown) || atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("lost answer: err=%v calls=%d", err, calls)
	}

	// Connection refused never reached the API: retried, then reported.
	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()
	_, err = NewTestClient("tok", closedURL).PublishPage("e", []byte("a"), PagePublishMeta{Name: "x"})
	if err == nil || errors.Is(err, errPublishUnknown) || !isDialError(err) {
		t.Fatalf("refused: err=%v", err)
	}

	atomic.StoreInt32(&calls, 0)
	srv4 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(409)
		_, _ = io.WriteString(w, `{"error":"pages_require_gateway"}`)
	}))
	defer srv4.Close()
	_, err = NewTestClient("tok", srv4.URL).PublishPage("e", []byte("a"), PagePublishMeta{Name: "x"})
	if err == nil || !strings.Contains(err.Error(), "pages_require_gateway") || atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("4xx: err=%v calls=%d", err, calls)
	}
	if _, err := NewTestClient("tok", srv4.URL).PublishPage("Bad Slug", nil, PagePublishMeta{Name: "x"}); err == nil {
		t.Fatal("invalid slug accepted")
	}
}

func TestPagesReadAndRemoveRequests(t *testing.T) {
	var method, path, query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, query = r.Method, r.URL.Path, r.URL.RawQuery
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/apps/e/pages":
			_, _ = io.WriteString(w, `{"pages":[{"name":"bilan","latest_version":2,"versions":2}],"gateway":true}`)
		case r.Method == "GET":
			_, _ = io.WriteString(w, `{"name":"bilan","latest_version":2,"versions":[{"version":2,"latest":true},{"version":1}]}`)
		default:
			_, _ = io.WriteString(w, `{"name":"bilan","removed_version":1,"page_removed":false}`)
		}
	}))
	defer srv.Close()
	c := NewTestClient("tok", srv.URL)
	l, err := c.ListPages("e")
	if err != nil || !l.Gateway || len(l.Pages) != 1 || l.Pages[0].LatestVersion != 2 {
		t.Fatalf("list = %+v %v", l, err)
	}
	v, err := c.ListPageVersions("e", "/pages/bilan/")
	if err != nil || path != "/v1/apps/e/pages/bilan" || len(v.Versions) != 2 || !v.Versions[0].Latest {
		t.Fatalf("versions = %+v %v path=%s", v, err, path)
	}
	r, err := c.RemovePage("e", "bilan", 1, false)
	if err != nil || method != "DELETE" || path != "/v1/apps/e/pages/bilan" || query != "version=1" || r.RemovedVersion != 1 {
		t.Fatalf("remove = %+v %v %s %s?%s", r, err, method, path, query)
	}
	if _, err := c.RemovePage("e", "bilan", 0, true); err != nil || query != "purge=true" {
		t.Fatalf("purge query = %q %v", query, err)
	}
	if _, err := c.RemovePage("e", "../x", 0, false); err == nil {
		t.Fatal("unsafe page name reached the URL")
	}
}
