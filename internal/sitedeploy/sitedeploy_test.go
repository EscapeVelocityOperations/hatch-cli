package sitedeploy

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	mrand "math/rand/v2"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

// B1 (h-bgdsq.6 review): a symlink inside the target must not publish a file
// that .hatchignore, the env rule or the .git default leave out. The artifact
// keeps such a link dead; the site skips it.
func TestBuild_SkipsSymlinkToExcludedFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "index.html", "<h1>hi</h1>")
	write(t, dir, "assets/app.js", "console.log(1)")
	write(t, dir, ".env", "SECRET=topsecret")
	write(t, dir, "config/.env.production", "SECRET=prod")
	write(t, dir, ".git/config", "[remote] url=https://tok@github.com/x")
	write(t, dir, "drafts/wip.html", "wip")
	write(t, dir, "drafts/deep/more.html", "more")
	write(t, dir, "notes.md", "notes")
	write(t, dir, ".hatchignore", "drafts/\n*.md\n")
	links := map[string]string{
		"robots.txt":        ".env",                   // env file
		"prod.txt":          "config/.env.production", // env file at depth
		"gitcfg.txt":        ".git/config",            // .git default
		"wip.html":          "drafts/wip.html",        // .hatchignore directory
		"more.html":         "drafts/deep/more.html",  // below an excluded directory
		"notes.txt":         "notes.md",               // .hatchignore pattern
		"assets/up-env.txt": "../.env",                // relative, from a subdirectory
		"chain1":            "chain2",                 // chain of links ending at an env file
		"chain2":            ".env",                   //
		"viadir.html":       "drafts-link/wip.html",   // through a link to an excluded directory
		"drafts-link":       "drafts",                 // (a link to a directory is skipped itself)
		"ok-link.html":      "index.html",             // control: a plain internal link stays published
		"assets/ok.js":      "../assets/app.js",       // control: relative internal link
		"assets/up-ok.html": "../index.html",          // control
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(dir, filepath.FromSlash(name))); err != nil {
			t.Fatal(err)
		}
	}

	m, err := Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	secrets := map[string]string{
		sum("SECRET=topsecret"):                      ".env",
		sum("SECRET=prod"):                           "config/.env.production",
		sum("[remote] url=https://tok@github.com/x"): ".git/config",
		sum("wip"):   "drafts/wip.html",
		sum("more"):  "drafts/deep/more.html",
		sum("notes"): "notes.md",
	}
	for _, f := range m.Files {
		if from, leaked := secrets[f.Sha256]; leaked {
			t.Errorf("LEAK: %s is published with the content of %s", f.Path, from)
		}
	}
	got := strings.Join(paths(m), ",")
	want := ".hatchignore,assets/app.js,assets/ok.js,assets/up-ok.html,index.html,ok-link.html"
	if got != want {
		t.Fatalf("published = %s\nwant       %s", got, want)
	}
	// Every refused link is reported, and none ends in the blobs to upload.
	skipped := map[string]bool{}
	for _, s := range m.Skipped {
		skipped[filepath.ToSlash(s)] = true
	}
	for _, name := range []string{"robots.txt", "prod.txt", "gitcfg.txt", "wip.html", "more.html", "notes.txt", "assets/up-env.txt", "chain1", "chain2", "viadir.html", "drafts-link"} {
		if !skipped[name] {
			t.Errorf("%s not reported in Skipped (%v)", name, m.Skipped)
		}
	}
	for sha := range secrets {
		if _, held := m.source[sha]; held {
			t.Errorf("blob %s... of an excluded file is in the upload set", sha[:12])
		}
	}
}

// The same rule when the link itself is the excluded name (an env file that
// is a symlink): the link path is matched first.
func TestBuild_SkipsExcludedSymlinkName(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "index.html", "<h1>hi</h1>")
	write(t, dir, "real.txt", "real")
	if err := os.Symlink("real.txt", filepath.Join(dir, ".env")); err != nil {
		t.Fatal(err)
	}
	m, err := Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(paths(m), ","); got != "index.html,real.txt" {
		t.Fatalf("published = %s", got)
	}
}

// N1: with a relative --deploy-target (dist), a link to a file inside the
// target is published, as with an absolute one.
func TestBuild_InternalSymlinkWithRelativeTarget(t *testing.T) {
	root := t.TempDir()
	write(t, root, "dist/index.html", "<h1>hi</h1>")
	write(t, root, "dist/.env", "SECRET=1")
	if err := os.Symlink("index.html", filepath.Join(root, "dist", "home.html")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".env", filepath.Join(root, "dist", "env.txt")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	for _, target := range []string{"dist", "./dist", filepath.Join(root, "dist")} {
		m, err := Build(target)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(paths(m), ","); got != "home.html,index.html" {
			t.Errorf("Build(%q) published %s, want home.html,index.html", target, got)
		}
		if len(m.Skipped) != 1 || m.Skipped[0] != "env.txt" {
			t.Errorf("Build(%q) skipped %v, want [env.txt]", target, m.Skipped)
		}
	}
}

// A target reached through a linked parent directory (macOS /tmp, a linked
// workspace) behaves the same: the target's real path is the reference.
func TestBuild_TargetBehindSymlink(t *testing.T) {
	root := t.TempDir()
	write(t, root, "real/site/index.html", "<h1>hi</h1>")
	write(t, root, "real/site/.env", "SECRET=1")
	if err := os.Symlink("index.html", filepath.Join(root, "real", "site", "home.html")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".env", filepath.Join(root, "real", "site", "env.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	m, err := Build(filepath.Join(root, "linked", "site"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(paths(m), ","); got != "home.html,index.html" {
		t.Fatalf("published = %s", got)
	}
}

// countingClient counts the manifest posts and fails them all with err.
type countingClient struct {
	err   error
	posts int
}

func (c *countingClient) IngestSiteManifest(string, []api.SiteFile) (*api.SiteIngestResult, error) {
	c.posts++
	return nil, c.err
}
func (c *countingClient) UploadSiteBlobs(string, io.Reader) (*api.SiteBlobsResult, error) {
	return nil, errors.New("unexpected upload")
}
func (c *countingClient) PublishSite(string, []api.SiteFile) (*api.SitePublishResult, error) {
	return nil, errors.New("unexpected publish")
}

func urlErr(err error) error { return &url.Error{Op: "Post", URL: "https://api.example/x", Err: err} }

// N4: a failure that a new try cannot fix is not retried (no ~14 s lost on a
// bad certificate or an unknown host); a broken connection still is.
func TestTransient_PermanentTransportErrorsAreNotRetried(t *testing.T) {
	// Real failures: a closed port, a certificate the client does not trust.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedAddr := ln.Addr().String()
	_ = ln.Close()
	_, refused := http.Get("http://" + closedAddr)
	tlsSrv := httptest.NewTLSServer(http.NotFoundHandler())
	defer tlsSrv.Close()
	_, badCert := (&http.Client{}).Get(tlsSrv.URL)
	if refused == nil || badCert == nil {
		t.Fatalf("setup: refused=%v badCert=%v", refused, badCert)
	}

	permanent := map[string]error{
		"connection refused (real)":  refused,
		"unknown certificate (real)": badCert,
		"dns no such host":           urlErr(&net.DNSError{Err: "no such host", Name: "api.example", IsNotFound: true}),
		"x509 hostname":              urlErr(x509.HostnameError{Host: "api.example"}),
		"x509 unknown authority":     urlErr(x509.UnknownAuthorityError{}),
		"tls record header":          urlErr(tls.RecordHeaderError{Msg: "first record does not look like a TLS handshake"}),
		"bad address":                urlErr(&net.AddrError{Err: "invalid port", Addr: "x:y"}),
	}
	for name, err := range permanent {
		if transient(err) {
			t.Errorf("%s: transient = true, want false", name)
		}
		c := &countingClient{err: err}
		_, got := Publish(c, "site-a1b2", &Manifest{Files: []api.SiteFile{{Path: "a", Sha256: sum("a"), Size: 1}}}, nil)
		if got == nil || c.posts != 1 {
			t.Errorf("%s: %d manifest posts (err %v), want 1 and an error", name, c.posts, got)
		}
	}

	transientErrs := map[string]error{
		"eof":              urlErr(io.EOF),
		"unexpected eof":   urlErr(io.ErrUnexpectedEOF),
		"connection reset": urlErr(&net.OpError{Op: "read", Err: os.NewSyscallError("read", syscall.ECONNRESET)}),
		"broken pipe":      urlErr(&net.OpError{Op: "write", Err: os.NewSyscallError("write", syscall.EPIPE)}),
		"dns temporary":    urlErr(&net.DNSError{Err: "server misbehaving", Name: "api.example", IsTemporary: true}),
	}
	for name, err := range transientErrs {
		if !transient(err) {
			t.Errorf("%s: transient = false, want true", name)
		}
		c := &countingClient{err: err}
		_, got := Publish(c, "site-a1b2", &Manifest{Files: []api.SiteFile{{Path: "a", Sha256: sum("a"), Size: 1}}}, nil)
		if got == nil || c.posts != maxUploads+1 {
			t.Errorf("%s: %d manifest posts (err %v), want %d", name, c.posts, got, maxUploads+1)
		}
	}
}

// Only the refusals an artifact deploy does not make are "site only": the
// preview pre-check of hatch deploy lets those through.
func TestIsSiteOnly(t *testing.T) {
	empty := t.TempDir()
	if _, err := Build(empty); err == nil || !IsSiteOnly(err) {
		t.Errorf("empty target: err = %v, want a site-only refusal", err)
	}

	env := t.TempDir()
	write(t, env, "index.html", "x")
	write(t, env, ".hatchignore", "!.env\n")
	if _, err := Build(env); err == nil || IsSiteOnly(err) {
		t.Errorf("env negation: err = %v, want a refusal the artifact makes too", err)
	}

	unreadable := t.TempDir()
	write(t, unreadable, ".hatchignore", "x\n")
	if err := os.Chmod(filepath.Join(unreadable, ".hatchignore"), 0); err != nil {
		t.Fatal(err)
	}
	if f, err := os.Open(filepath.Join(unreadable, ".hatchignore")); err == nil { // running as root
		_ = f.Close()
	} else if _, err := Build(unreadable); err == nil || IsSiteOnly(err) {
		t.Errorf("unreadable .hatchignore: err = %v, want a refusal the artifact makes too", err)
	}

	if IsSiteOnly(errors.New("boom")) || IsSiteOnly(nil) {
		t.Error("IsSiteOnly is true for a foreign error")
	}
}
