// Package sitetest is a simulated hatch-api for tests of site deploys: the
// routes of the site contract (hatch-api docs/plans/h-bgdsq-9-site-ingest-relay.md)
// with a blob store behind them, plus the few routes a deploy touches
// (deployment history, artifact upload, egg status, previews). It checks what
// the real API and hatch-control check on the wire and counts every byte.
package sitetest

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
)

var shaRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// File is a manifest entry as the server reads it.
type File struct {
	Path   string `json:"path"`
	Sha256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Server simulates hatch-api for one egg. Set the knobs before the deploy;
// read the counters after it. All fields are guarded by Mu.
type Server struct {
	*httptest.Server
	Mu sync.Mutex

	Slug string

	// Knobs.
	Disabled      bool   // every site route answers 404 sites_disabled
	HasSite       bool   // the deployment history holds a "kind":"site" row
	PreviewKind   string // "kind" of the preview answer ("" = field absent)
	MaxFileBytes  int64  // 0 = no bound (413 site_file_too_large)
	MaxBytes      int64  // per publication (413 site_too_large)
	Quota         int64  // stored + new bytes per egg (413 site_quota_exceeded)
	CutUploads    int    // the next N blob uploads store one blob, then answer 400 site_upload_interrupted
	FailUploads   int    // the next N blob uploads answer 502 site_ingest_failed without storing
	FailManifests int    // the next N manifest posts answer 502 site_ingest_failed
	HistoryStatus int    // != 0: GET deployments answers this status

	// Store: sha256 -> content, as control keeps blobs.
	Blobs map[string][]byte

	// Counters.
	HistoryGets       int
	DisabledHits      int // site requests answered 404 sites_disabled
	ManifestPosts     int
	ManifestWireBytes int64 // bodies of the manifest posts
	BlobPosts         int
	BlobWireBytes     int64 // bodies of the blob uploads, as received (gzip)
	BlobFileBytes     int64 // content of the blobs stored by the uploads
	BlobEntries       int   // tar entries received
	PublishPosts      int
	PublishWireBytes  int64
	Versions          [][]File // the published manifests, in order
	ArtifactUploads   int
	ArtifactBytes     int64
	PreviewPosts      int
}

// New starts the server for slug and closes it at the end of the test.
func New(t testing.TB, slug string) *Server {
	t.Helper()
	s := &Server{Slug: slug, Blobs: map[string][]byte{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func refuse(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	rest, ok := strings.CutPrefix(r.URL.Path, "/v1/apps/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	slug, tail, _ := strings.Cut(rest, "/")
	isSite := tail == "site" || strings.HasPrefix(tail, "site/")
	if isSite && s.Disabled {
		s.DisabledHits++
		refuse(w, http.StatusNotFound, "sites_disabled")
		return
	}
	if r.Header.Get("Authorization") == "" {
		refuse(w, http.StatusUnauthorized, "authorization required")
		return
	}
	if tail == "previews" && r.Method == http.MethodPost {
		s.previews(w, slug)
		return
	}
	if slug != s.Slug {
		refuse(w, http.StatusNotFound, "app not found")
		return
	}
	switch {
	case r.Method == http.MethodGet && tail == "deployments":
		s.HistoryGets++
		if s.HistoryStatus != 0 {
			refuse(w, s.HistoryStatus, "internal server error")
			return
		}
		rows := []map[string]any{{"id": "d-image", "status": "live", "image_url": "registry/x:1"}}
		if s.HasSite {
			rows = append([]map[string]any{{"id": "d-site", "status": "live", "image_url": "", "kind": "site", "site_version": 1}}, rows...)
		}
		writeJSON(w, http.StatusOK, rows)
	case r.Method == http.MethodGet && tail == "":
		writeJSON(w, http.StatusOK, map[string]string{"slug": slug, "status": "running"})
	case r.Method == http.MethodPost && tail == "artifact":
		n, _ := io.Copy(io.Discard, r.Body)
		s.ArtifactUploads++
		s.ArtifactBytes += n
		writeJSON(w, http.StatusOK, map[string]string{"status": "deploying"})
	case r.Method == http.MethodPost && tail == "site/ingest":
		ct := r.Header.Get("Content-Type")
		switch {
		case strings.HasPrefix(ct, "application/json"):
			s.manifest(w, r)
		case strings.HasPrefix(ct, "application/x-tar"):
			s.blobs(w, r)
		default:
			refuse(w, http.StatusUnsupportedMediaType, "site_unsupported_media_type")
		}
	case r.Method == http.MethodPost && tail == "site":
		s.publish(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) previews(w http.ResponseWriter, parent string) {
	s.PreviewPosts++
	out := map[string]any{"slug": s.Slug, "pr_number": 7, "url": "https://" + s.Slug + ".nest.test", "status": "running", "created": true}
	if s.PreviewKind != "" {
		out["kind"] = s.PreviewKind
	}
	writeJSON(w, http.StatusCreated, out)
}

// readManifest applies the manifest checks of the API (checkSiteManifest)
// and of control (path rules, one size per hash).
func (s *Server) readManifest(w http.ResponseWriter, r *http.Request, wire *int64) ([]File, map[string]int64, bool) {
	data, _ := io.ReadAll(r.Body)
	*wire += int64(len(data))
	var req struct {
		Files []File `json:"files"`
	}
	if err := json.Unmarshal(data, &req); err != nil || len(req.Files) == 0 {
		refuse(w, http.StatusBadRequest, "invalid_site_manifest")
		return nil, nil, false
	}
	sizeOf := map[string]int64{}
	paths := map[string]bool{}
	var total int64
	for _, f := range req.Files {
		if f.Path == "" || !shaRe.MatchString(f.Sha256) || f.Size < 0 || paths[f.Path] {
			refuse(w, http.StatusBadRequest, "invalid_site_manifest")
			return nil, nil, false
		}
		if strings.HasPrefix(f.Path, "/") || strings.Contains(f.Path, "\\") || strings.Contains("/"+f.Path+"/", "/../") {
			refuse(w, http.StatusBadRequest, "site_unsafe_path")
			return nil, nil, false
		}
		if prev, seen := sizeOf[f.Sha256]; seen && prev != f.Size {
			refuse(w, http.StatusBadRequest, "invalid_site_manifest")
			return nil, nil, false
		}
		if s.MaxFileBytes > 0 && f.Size > s.MaxFileBytes {
			refuse(w, http.StatusRequestEntityTooLarge, "site_file_too_large")
			return nil, nil, false
		}
		paths[f.Path] = true
		sizeOf[f.Sha256] = f.Size
		total += f.Size
	}
	if s.MaxBytes > 0 && total > s.MaxBytes {
		refuse(w, http.StatusRequestEntityTooLarge, "site_too_large")
		return nil, nil, false
	}
	if s.Quota > 0 {
		var stored, extra int64
		for _, b := range s.Blobs {
			stored += int64(len(b))
		}
		for sum, size := range sizeOf {
			if b, ok := s.Blobs[sum]; !ok || int64(len(b)) != size {
				extra += size
			}
		}
		if stored+extra > s.Quota {
			refuse(w, http.StatusRequestEntityTooLarge, "site_quota_exceeded")
			return nil, nil, false
		}
	}
	return req.Files, sizeOf, true
}

func (s *Server) manifest(w http.ResponseWriter, r *http.Request) {
	s.ManifestPosts++
	files, _, ok := s.readManifest(w, r, &s.ManifestWireBytes)
	if !ok {
		return
	}
	if s.FailManifests > 0 {
		s.FailManifests--
		refuse(w, http.StatusBadGateway, "site_ingest_failed")
		return
	}
	missing := []string{}
	seen := map[string]bool{}
	unique := 0
	var total, missingBytes int64
	for _, f := range files {
		total += f.Size
		if seen[f.Sha256] {
			continue
		}
		seen[f.Sha256] = true
		unique++
		if b, ok := s.Blobs[f.Sha256]; !ok || int64(len(b)) != f.Size {
			missing = append(missing, f.Sha256)
			missingBytes += f.Size
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"missing": missing, "files": len(files), "unique": unique, "bytes": total, "missing_bytes": missingBytes,
	})
}

type countingReader struct {
	r io.Reader
	n *int64
}

func (c countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	*c.n += int64(n)
	return n, err
}

func (s *Server) blobs(w http.ResponseWriter, r *http.Request) {
	s.BlobPosts++
	body := io.Reader(countingReader{r.Body, &s.BlobWireBytes})
	if s.FailUploads > 0 {
		s.FailUploads--
		_, _ = io.Copy(io.Discard, body)
		refuse(w, http.StatusBadGateway, "site_ingest_failed")
		return
	}
	switch r.Header.Get("Content-Encoding") {
	case "", "identity":
	case "gzip":
		gz, err := gzip.NewReader(body)
		if err != nil {
			refuse(w, http.StatusBadRequest, "site_invalid_archive")
			return
		}
		body = gz
	default:
		refuse(w, http.StatusUnsupportedMediaType, "site_unsupported_media_type")
		return
	}
	cut := s.CutUploads > 0
	if cut {
		s.CutUploads--
	}
	tr := tar.NewReader(body)
	created := []map[string]string{}
	present := 0
	var written int64
	seen := map[string]bool{}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			refuse(w, http.StatusBadRequest, "site_invalid_archive")
			return
		}
		if hdr.Typeflag != tar.TypeReg {
			refuse(w, http.StatusBadRequest, "site_unsafe_entry")
			return
		}
		if !shaRe.MatchString(hdr.Name) {
			refuse(w, http.StatusBadRequest, "site_invalid_blob_name")
			return
		}
		if seen[hdr.Name] {
			refuse(w, http.StatusBadRequest, "site_duplicate_blob")
			return
		}
		seen[hdr.Name] = true
		s.BlobEntries++
		if b, ok := s.Blobs[hdr.Name]; ok && int64(len(b)) == hdr.Size {
			present++
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			refuse(w, http.StatusBadRequest, "site_upload_interrupted")
			return
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != hdr.Name {
			refuse(w, http.StatusBadRequest, "site_blob_hash_mismatch")
			return
		}
		s.Blobs[hdr.Name] = data
		s.BlobFileBytes += int64(len(data))
		written += int64(len(data))
		created = append(created, map[string]string{"sha256": hdr.Name})
		if cut {
			// Blobs written before a cut are kept (contract): stop here.
			refuse(w, http.StatusBadRequest, "site_upload_interrupted")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"created": created, "present": present, "bytes_written": written})
}

func (s *Server) publish(w http.ResponseWriter, r *http.Request) {
	s.PublishPosts++
	files, _, ok := s.readManifest(w, r, &s.PublishWireBytes)
	if !ok {
		return
	}
	// The real API does not check presence; the simulation does, to prove
	// the client never confirms before every blob is stored.
	for _, f := range files {
		if b, ok := s.Blobs[f.Sha256]; !ok || int64(len(b)) != f.Size {
			refuse(w, http.StatusConflict, "test_blob_missing_at_publish")
			return
		}
	}
	s.Versions = append(s.Versions, files)
	s.HasSite = true
	var total int64
	for _, f := range files {
		total += f.Size
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"version": len(s.Versions), "version_id": fmt.Sprintf("v-%d", len(s.Versions)), "files": len(files), "bytes": total,
		"created_at": "2026-11-17T00:00:00Z", "url": "https://" + s.Slug + ".nest.test/", "deployment_id": "d-" + s.Slug,
	})
}
