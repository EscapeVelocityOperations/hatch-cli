package sitedeploy

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	mrand "math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/sitedeploy/sitetest"
)

func init() { RetryDelay = 0 }

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func paths(m *Manifest) []string {
	var out []string
	for _, f := range m.Files {
		out = append(out, f.Path)
	}
	return out
}

func newServer(t *testing.T) (*sitetest.Server, *api.Client) {
	srv := sitetest.New(t, "site-a1b2")
	srv.HasSite = true
	return srv, api.NewTestClient("tok", srv.URL)
}

// The manifest is the artifact's file set: .hatchignore and the safety
// defaults apply, env files never go, a symlink to a file inside the target
// is published with its content, one outside is skipped.
func TestBuild_RespectsHatchignoreAndDefaults(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "index.html", "<h1>hi</h1>")
	write(t, dir, "assets/app.js", "console.log(1)")
	write(t, dir, "drafts/wip.html", "wip")
	write(t, dir, "notes.md", "notes")
	write(t, dir, ".env", "SECRET=1")
	write(t, dir, "config/.env.production", "SECRET=2")
	write(t, dir, ".env.example", "SECRET=")
	write(t, dir, ".hatch.toml", "[app]\nslug = \"x\"\n")
	write(t, dir, ".git/config", "[core]")
	write(t, dir, ".hatchignore", "drafts/\n*.md\n")
	outside := t.TempDir()
	write(t, outside, "secret.txt", "outside")
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "leak.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("index.html", filepath.Join(dir, "home.html")); err != nil {
		t.Fatal(err)
	}

	m, err := Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(paths(m), ",")
	want := ".env.example,.hatchignore,assets/app.js,home.html,index.html"
	if got != want {
		t.Fatalf("files = %s, want %s", got, want)
	}
	for _, f := range m.Files {
		if f.Path == "home.html" && f.Sha256 != sum("<h1>hi</h1>") {
			t.Fatalf("symlink inside the target must carry its target's content")
		}
	}
	if m.Unique() != 4 { // home.html and index.html share a blob
		t.Fatalf("unique = %d, want 4", m.Unique())
	}
	if strings.Join(m.Skipped, ",") != "leak.txt" {
		t.Fatalf("skipped = %v, want leak.txt", m.Skipped)
	}
	for _, e := range []string{".env", "config/.env.production", ".hatch.toml", ".git/", "drafts/", "notes.md"} {
		if !strings.Contains(","+strings.Join(m.Excluded, ",")+",", ","+e+",") {
			t.Fatalf("excluded = %v, missing %s", m.Excluded, e)
		}
	}
}

func TestBuild_RefusesEnvNegationAndEmptyTarget(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "index.html", "x")
	write(t, dir, ".hatchignore", "!.env\n")
	if _, err := Build(dir); err == nil || !strings.Contains(err.Error(), "re-includes env files (!.env)") {
		t.Fatalf("err = %v, want env negation refusal", err)
	}

	empty := t.TempDir()
	write(t, empty, ".hatchignore", "*\n")
	if _, err := Build(empty); err == nil || !strings.Contains(err.Error(), "nothing to publish") {
		t.Fatalf("err = %v, want nothing to publish", err)
	}
}

// Names go out in NFC; two names equal once normalized are refused here,
// not by the platform after an upload.
func TestBuild_NormalizesNamesToNFC(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "café.html", "nfd") // e + combining acute
	m, err := Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Files[0].Path != "café.html" {
		t.Fatalf("path = %q, want NFC", m.Files[0].Path)
	}
	write(t, dir, "café.html", "nfc")
	if _, err := Build(dir); err == nil || !strings.Contains(err.Error(), "same file name once normalized") {
		t.Fatalf("err = %v, want normalization clash", err)
	}
}

// First deploy: manifest, every blob once (duplicates share one), the check
// replay, then the confirmation with the full list.
func TestPublish_MigratedEggUploadsBlobsThenConfirms(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "index.html", "<h1>v1</h1>")
	write(t, dir, "a.css", "body{}")
	write(t, dir, "copy.css", "body{}")
	srv, c := newServer(t)

	m, err := Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Publish(c, srv.Slug, m, nil)
	if err != nil {
		t.Fatal(err)
	}
	if srv.ManifestPosts != 2 || srv.BlobPosts != 1 || srv.PublishPosts != 1 {
		t.Fatalf("manifest/blob/publish posts = %d/%d/%d, want 2/1/1", srv.ManifestPosts, srv.BlobPosts, srv.PublishPosts)
	}
	if srv.BlobEntries != 2 || res.UploadedFiles != 2 {
		t.Fatalf("blobs sent = %d (result %d), want 2 (copy.css shares a.css's blob)", srv.BlobEntries, res.UploadedFiles)
	}
	if len(srv.Versions) != 1 || len(srv.Versions[0]) != 3 {
		t.Fatalf("published versions = %v", srv.Versions)
	}
	if res.Publish.Version != 1 || res.Publish.URL == "" || res.SentBytes != srv.BlobWireBytes {
		t.Fatalf("result = %+v (server saw %d wire bytes)", res, srv.BlobWireBytes)
	}
}

// Only what the platform lacks goes: an unchanged redeploy uploads nothing,
// a one-file change uploads that file.
func TestPublish_SendsOnlyMissingBlobs(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "index.html", "<h1>v1</h1>")
	write(t, dir, "big.bin", strings.Repeat("x", 100_000))
	srv, c := newServer(t)
	deploy := func() *Result {
		t.Helper()
		m, err := Build(dir)
		if err != nil {
			t.Fatal(err)
		}
		res, err := Publish(c, srv.Slug, m, nil)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	deploy()

	before := srv.BlobPosts
	res := deploy()
	if srv.BlobPosts != before || res.UploadedFiles != 0 || res.SentBytes != 0 {
		t.Fatalf("unchanged redeploy uploaded %d files in %d posts", res.UploadedFiles, srv.BlobPosts-before)
	}

	write(t, dir, "index.html", "<h1>v3</h1>")
	entries := srv.BlobEntries
	res = deploy()
	if srv.BlobEntries-entries != 1 || res.UploadedBytes != int64(len("<h1>v3</h1>")) {
		t.Fatalf("one-file change sent %d blobs, %d bytes", srv.BlobEntries-entries, res.UploadedBytes)
	}
	if len(srv.Versions) != 3 {
		t.Fatalf("versions = %d, want 3", len(srv.Versions))
	}
}

// Switch off: nothing is sent past the first manifest, the caller falls back.
func TestPublish_SitesDisabled(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "index.html", "x")
	srv, c := newServer(t)
	srv.Disabled = true
	m, _ := Build(dir)
	if _, err := Publish(c, srv.Slug, m, nil); !errors.Is(err, ErrSitesDisabled) {
		t.Fatalf("err = %v, want ErrSitesDisabled", err)
	}
	if srv.BlobPosts != 0 || srv.PublishPosts != 0 {
		t.Fatalf("blob/publish posts = %d/%d, want 0/0", srv.BlobPosts, srv.PublishPosts)
	}
}

// 413 refusals stop the deploy before any upload, with the code and what it means.
func TestPublish_413RefusalsBeforeUpload(t *testing.T) {
	cases := []struct {
		name string
		set  func(*sitetest.Server)
		code string
		hint string
	}{
		{"quota", func(s *sitetest.Server) { s.Quota = 10 }, "site_quota_exceeded", "quota"},
		{"file", func(s *sitetest.Server) { s.MaxFileBytes = 10 }, "site_file_too_large", "larger than the platform allows"},
		{"site", func(s *sitetest.Server) { s.MaxBytes = 10 }, "site_too_large", "site is larger"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, "index.html", strings.Repeat("x", 64))
			srv, c := newServer(t)
			tc.set(srv)
			m, _ := Build(dir)
			_, err := Publish(c, srv.Slug, m, nil)
			var se *api.SiteError
			if !errors.As(err, &se) || se.Status != 413 || se.Code != tc.code {
				t.Fatalf("err = %v, want 413 %s", err, tc.code)
			}
			if !strings.Contains(err.Error(), "413") || !strings.Contains(err.Error(), tc.code) || !strings.Contains(err.Error(), tc.hint) {
				t.Fatalf("message %q lacks status, code or hint", err)
			}
			if srv.ManifestPosts != 1 || srv.BlobPosts != 0 || srv.PublishPosts != 0 {
				t.Fatalf("manifest/blob/publish posts = %d/%d/%d, want 1/0/0", srv.ManifestPosts, srv.BlobPosts, srv.PublishPosts)
			}
		})
	}
}

// A cut or a 502 is resumed: the manifest is posted again and only what is
// still missing goes.
func TestPublish_ResumesAfterTransientFailures(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		write(t, dir, fmt.Sprintf("f%d.txt", i), strings.Repeat(fmt.Sprint(i), 50_000))
	}
	srv, c := newServer(t)
	srv.CutUploads = 1 // first upload stores one blob then is cut
	srv.FailManifests = 1
	m, _ := Build(dir)
	res, err := Publish(c, srv.Slug, m, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(srv.Blobs) != 5 || srv.PublishPosts != 1 {
		t.Fatalf("stored %d blobs, %d publications", len(srv.Blobs), srv.PublishPosts)
	}
	if srv.BlobFileBytes != 5*50_000 {
		t.Fatalf("server stored %d bytes, want each blob once (%d)", srv.BlobFileBytes, 5*50_000)
	}
	if res.UploadedFiles != 4 { // the cut upload stored one, the resumed one the other four
		t.Fatalf("uploaded files = %d, want 4 after the cut", res.UploadedFiles)
	}

	// 502 on the upload: retried.
	write(t, dir, "f0.txt", "changed")
	srv.FailUploads = 1
	m, _ = Build(dir)
	if _, err := Publish(c, srv.Slug, m, nil); err != nil {
		t.Fatal(err)
	}
	if srv.PublishPosts != 2 {
		t.Fatalf("publications = %d, want 2", srv.PublishPosts)
	}
}

// A platform that keeps failing ends the deploy with an error, no publication.
func TestPublish_GivesUpAfterRepeatedFailures(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "index.html", "x")
	srv, c := newServer(t)
	srv.FailUploads = 100
	m, _ := Build(dir)
	_, err := Publish(c, srv.Slug, m, nil)
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("err = %v, want the 502", err)
	}
	if srv.PublishPosts != 0 || srv.BlobPosts != maxUploads {
		t.Fatalf("blob/publish posts = %d/%d, want %d/0", srv.BlobPosts, srv.PublishPosts, maxUploads)
	}
}

// T-506 measure: a redeploy that changes 5 % of the files sends ≈ 5 % of the
// site's bytes, against 100 % for an artifact. The content is random
// (incompressible), so gzip neither helps nor hides anything; the manifest
// posts (two before the upload, one to confirm) are counted too.
func TestPublish_FivePercentChangeSendsAboutFivePercent(t *testing.T) {
	const files = 400
	dir := t.TempDir()
	rng := mrand.New(mrand.NewPCG(1, 2))
	sizes := make([]int, files)
	writeRandom := func(i int) {
		b := make([]byte, sizes[i])
		_, _ = rand.Read(b)
		write(t, dir, fmt.Sprintf("assets/%03d/file-%04d.bin", i%40, i), string(b))
	}
	for i := range sizes {
		sizes[i] = 32<<10 + rng.IntN(64<<10) // 32 to 96 KiB
		writeRandom(i)
	}
	srv, c := newServer(t)
	m, err := Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Publish(c, srv.Slug, m, nil); err != nil {
		t.Fatal(err)
	}

	var changedBytes int64
	for i := 0; i < files; i += 20 { // every 20th file: 5 %
		writeRandom(i)
		changedBytes += int64(sizes[i])
	}
	srv.ManifestWireBytes, srv.BlobWireBytes, srv.PublishWireBytes = 0, 0, 0
	m, err = Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Publish(c, srv.Slug, m, nil)
	if err != nil {
		t.Fatal(err)
	}
	site := float64(m.Bytes)
	blobs := float64(srv.BlobWireBytes) / site
	total := float64(srv.ManifestWireBytes+srv.BlobWireBytes+srv.PublishWireBytes) / site
	t.Logf("site %d files, %d bytes; changed %d files, %d bytes (%.2f %%); sent: blobs %d bytes (%.2f %%), file lists %d bytes, total %.2f %%",
		files, m.Bytes, res.UploadedFiles, changedBytes, 100*float64(changedBytes)/site,
		srv.BlobWireBytes, 100*blobs, srv.ManifestWireBytes+srv.PublishWireBytes, 100*total)
	if res.UploadedFiles != files/20 {
		t.Fatalf("uploaded %d files, want %d", res.UploadedFiles, files/20)
	}
	if blobs < 0.04 || blobs > 0.065 {
		t.Fatalf("blob bytes = %.2f %% of the site, want ≈ 5 %%", 100*blobs)
	}
	if total > 0.065 {
		t.Fatalf("total bytes = %.2f %% of the site, want ≤ 6.5 %%", 100*total)
	}
}
