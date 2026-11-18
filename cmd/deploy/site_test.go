package deploy

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/exitcode"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/sitedeploy"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/sitedeploy/sitetest"
)

// The mock of the other tests is never a site egg: HasSite is false and
// the site calls fail, so those tests prove the artifact path is untouched.
func (m *mockAPIClient) HasSite(slug string) (bool, error) { return false, nil }

func (m *mockAPIClient) IngestSiteManifest(string, []api.SiteFile) (*api.SiteIngestResult, error) {
	return nil, errors.New("unexpected site call")
}

func (m *mockAPIClient) UploadSiteBlobs(string, io.Reader) (*api.SiteBlobsResult, error) {
	return nil, errors.New("unexpected site call")
}

func (m *mockAPIClient) PublishSite(string, []api.SiteFile) (*api.SitePublishResult, error) {
	return nil, errors.New("unexpected site call")
}

// siteFixture points hatch deploy at a simulated hatch-api (real HTTP, real
// api.Client) and returns a deploy target holding a small site.
func siteFixture(t *testing.T) (*sitetest.Server, string) {
	t.Helper()
	srv := sitetest.New(t, "site-a1b2")
	old := sitedeploy.RetryDelay
	sitedeploy.RetryDelay = 0
	t.Cleanup(func() { sitedeploy.RetryDelay = old; resetDeployState() })
	now := time.Unix(0, 0)
	deps = &Deps{
		GetToken: func() (string, error) { return "tok", nil },
		NewAPIClient: func(token string) APIClient {
			return &realAPIClient{client: api.NewTestClient(token, srv.URL)}
		},
		Confirm: func(string) bool { return false },
		Sleep:   func(d time.Duration) { now = now.Add(d) },
		Now:     func() time.Time { return now },
	}
	dir := t.TempDir()
	for name, body := range map[string]string{
		"index.html":    "<h1>home</h1>",
		"about.html":    "<h1>about</h1>",
		"assets/app.js": strings.Repeat("console.log(1);\n", 2000),
		".env":          "SECRET=1",
	} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return srv, dir
}

func runSiteDeploy(t *testing.T, dir, slug, runtime, preview string) (map[string]any, error) {
	t.Helper()
	var err error
	out := captureOutput(func() {
		err = RunArtifactDeploy(ArtifactDeployConfig{
			Token: "tok", AppSlug: slug, DeployTarget: dir, Runtime: runtime,
			StartCommand: map[bool]string{true: "", false: "node index.html"}[runtime == "static"],
			PreviewRef:   preview, JSONOutput: true,
		})
	})
	var fields map[string]any
	if jerr := json.Unmarshal([]byte(strings.TrimSpace(out)), &fields); jerr != nil {
		t.Fatalf("stdout is not one JSON line: %q (%v)", out, jerr)
	}
	return fields, err
}

// Migrated egg: file list, the missing blobs only, confirmation; no artifact.
func TestDeploySite_MigratedEggPublishesChangedFilesOnly(t *testing.T) {
	srv, dir := siteFixture(t)
	srv.HasSite = true

	fields, err := runSiteDeploy(t, dir, "site-a1b2", "static", "")
	if err != nil {
		t.Fatal(err)
	}
	if fields["kind"] != "site" || fields["status"] != "live" || fields["site_version"] != float64(1) || fields["uploaded_files"] != float64(3) {
		t.Fatalf("json = %v", fields)
	}
	if srv.ArtifactUploads != 0 || srv.PublishPosts != 1 || srv.BlobEntries != 3 {
		t.Fatalf("artifact uploads %d, publications %d, blobs %d; want 0, 1, 3", srv.ArtifactUploads, srv.PublishPosts, srv.BlobEntries)
	}
	for _, f := range srv.Versions[0] {
		if f.Path == ".env" {
			t.Fatal(".env was published")
		}
	}

	// One file changed: one blob goes.
	if err := os.WriteFile(filepath.Join(dir, "about.html"), []byte("<h1>about v2</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := srv.BlobEntries
	fields, err = runSiteDeploy(t, dir, "site-a1b2", "static", "")
	if err != nil {
		t.Fatal(err)
	}
	if srv.BlobEntries-entries != 1 || fields["uploaded_files"] != float64(1) || fields["site_version"] != float64(2) {
		t.Fatalf("second deploy sent %d blobs, json %v", srv.BlobEntries-entries, fields)
	}
}

// Not migrated, or sites switched off on the platform: the artifact path, as before.
func TestDeploySite_FallsBackToArtifact(t *testing.T) {
	t.Run("egg without site", func(t *testing.T) {
		srv, dir := siteFixture(t)
		fields, err := runSiteDeploy(t, dir, "site-a1b2", "static", "")
		if err != nil {
			t.Fatal(err)
		}
		if srv.HistoryGets != 1 || srv.ManifestPosts != 0 || srv.ArtifactUploads != 1 || fields["status"] != "running" || fields["kind"] != nil {
			t.Fatalf("history %d, manifests %d, artifacts %d, json %v", srv.HistoryGets, srv.ManifestPosts, srv.ArtifactUploads, fields)
		}
	})
	t.Run("404 sites_disabled", func(t *testing.T) {
		srv, dir := siteFixture(t)
		srv.HasSite = true
		srv.Disabled = true
		fields, err := runSiteDeploy(t, dir, "site-a1b2", "static", "")
		if err != nil {
			t.Fatal(err)
		}
		if srv.DisabledHits != 1 || srv.BlobPosts != 0 || srv.PublishPosts != 0 || srv.ArtifactUploads != 1 || fields["status"] != "running" {
			t.Fatalf("sites_disabled answers %d, blobs %d, publications %d, artifacts %d, json %v",
				srv.DisabledHits, srv.BlobPosts, srv.PublishPosts, srv.ArtifactUploads, fields)
		}
	})
	t.Run("other runtime never asks", func(t *testing.T) {
		srv, dir := siteFixture(t)
		srv.HasSite = true
		if err := os.WriteFile(filepath.Join(dir, "server.js"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		var err error
		captureOutput(func() {
			err = RunArtifactDeploy(ArtifactDeployConfig{Token: "tok", AppSlug: "site-a1b2", DeployTarget: dir,
				Runtime: "node", StartCommand: "node server.js", JSONOutput: true})
		})
		if err != nil {
			t.Fatal(err)
		}
		if srv.HistoryGets != 0 || srv.ManifestPosts != 0 || srv.ArtifactUploads != 1 {
			t.Fatalf("history %d, manifests %d, artifacts %d", srv.HistoryGets, srv.ManifestPosts, srv.ArtifactUploads)
		}
	})
	t.Run("egg named by .hatch.toml", func(t *testing.T) {
		srv, dir := siteFixture(t)
		srv.HasSite = true
		writeParentHatchToml(t, "site-a1b2")
		fields, err := runSiteDeploy(t, dir, "", "static", "")
		if err != nil {
			t.Fatal(err)
		}
		if fields["kind"] != "site" || srv.ArtifactUploads != 0 {
			t.Fatalf("json %v, artifacts %d", fields, srv.ArtifactUploads)
		}
	})
}

// A 413 is printed with its code and meaning and exits 1 (outside a TTY too:
// the exit code is attached, h-59ej); nothing is uploaded.
func TestDeploySite_413ExitsNonZero(t *testing.T) {
	srv, dir := siteFixture(t)
	srv.HasSite = true
	srv.Quota = 100

	fields, err := runSiteDeploy(t, dir, "site-a1b2", "static", "")
	if err == nil {
		t.Fatal("want an error")
	}
	if code, ok := exitcode.From(err); !ok || code != ExitDeployError {
		t.Fatalf("exit code = %d (%v), want %d", code, ok, ExitDeployError)
	}
	msg, _ := fields["error"].(string)
	if fields["status"] != "error" || fields["slug"] != "site-a1b2" || !strings.Contains(msg, "413") || !strings.Contains(msg, "site_quota_exceeded") {
		t.Fatalf("json = %v", fields)
	}
	if srv.BlobPosts != 0 || srv.PublishPosts != 0 || srv.ArtifactUploads != 0 {
		t.Fatalf("blobs %d, publications %d, artifacts %d; want none", srv.BlobPosts, srv.PublishPosts, srv.ArtifactUploads)
	}
}

// The history cannot be read: stop, rather than build an image for an egg
// that may be a site.
func TestDeploySite_HistoryErrorStops(t *testing.T) {
	srv, dir := siteFixture(t)
	srv.HistoryStatus = 500
	_, err := runSiteDeploy(t, dir, "site-a1b2", "static", "")
	if err == nil || !strings.Contains(err.Error(), "checking whether egg site-a1b2 serves a Hatch site") {
		t.Fatalf("err = %v", err)
	}
	if code, _ := exitcode.From(err); code != ExitDeployError || srv.ArtifactUploads != 0 {
		t.Fatalf("exit %d, artifacts %d", code, srv.ArtifactUploads)
	}
}

// Previews follow the kind the API gives them (T-505 N3).
func TestDeploySite_PreviewKind(t *testing.T) {
	t.Run("site", func(t *testing.T) {
		srv, dir := siteFixture(t)
		srv.PreviewKind = "site"
		writeParentHatchToml(t, "parent")
		fields, err := runSiteDeploy(t, dir, "", "static", "pr-7")
		if err != nil {
			t.Fatal(err)
		}
		if fields["kind"] != "site" || fields["pr_number"] != float64(7) || srv.PublishPosts != 1 || srv.ArtifactUploads != 0 {
			t.Fatalf("json %v, publications %d, artifacts %d", fields, srv.PublishPosts, srv.ArtifactUploads)
		}
	})
	t.Run("image", func(t *testing.T) {
		srv, dir := siteFixture(t)
		srv.PreviewKind = "image"
		writeParentHatchToml(t, "parent")
		fields, err := runSiteDeploy(t, dir, "", "static", "pr-7")
		if err != nil {
			t.Fatal(err)
		}
		if fields["kind"] != nil || srv.ManifestPosts != 0 || srv.ArtifactUploads != 1 {
			t.Fatalf("json %v, manifests %d, artifacts %d", fields, srv.ManifestPosts, srv.ArtifactUploads)
		}
	})
}

// N2 (h-bgdsq.6 review): a static preview whose deploy target cannot be built
// (.hatchignore re-including an env file) fails before the preview egg is
// created, as it did when the artifact was built first, whatever the kind of
// the preview.
func TestDeploySite_PreviewTargetCheckedBeforeCreatingPreview(t *testing.T) {
	for _, kind := range []string{"site", "image"} {
		t.Run(kind, func(t *testing.T) {
			srv, dir := siteFixture(t)
			srv.PreviewKind = kind
			writeParentHatchToml(t, "parent")
			if err := os.WriteFile(filepath.Join(dir, ".hatchignore"), []byte("drafts/\n!.env\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			var err error
			captureOutput(func() {
				err = RunArtifactDeploy(ArtifactDeployConfig{
					Token: "tok", DeployTarget: dir, Runtime: "static", PreviewRef: "pr-7", JSONOutput: true,
				})
			})
			if err == nil || !strings.Contains(err.Error(), "env files") {
				t.Fatalf("err = %v, want the refusal of the re-included env file", err)
			}
			if srv.PreviewPosts != 0 || srv.ManifestPosts != 0 || srv.ArtifactUploads != 0 {
				t.Fatalf("previews %d, manifests %d, artifacts %d; want 0: nothing may reach the API", srv.PreviewPosts, srv.ManifestPosts, srv.ArtifactUploads)
			}
		})
	}
}

// The file list built before the preview is the one published: the preview
// of a site egg lists the deploy target once.
func TestDeploySite_PreviewSitePublishesTheListBuiltFirst(t *testing.T) {
	srv, dir := siteFixture(t)
	srv.PreviewKind = "site"
	writeParentHatchToml(t, "parent")
	if err := os.Symlink(".env", filepath.Join(dir, "env.txt")); err != nil {
		t.Fatal(err)
	}
	fields, err := runSiteDeploy(t, dir, "", "static", "pr-7")
	if err != nil {
		t.Fatal(err)
	}
	if fields["kind"] != "site" || srv.PreviewPosts != 1 || srv.PublishPosts != 1 {
		t.Fatalf("json %v, previews %d, publications %d", fields, srv.PreviewPosts, srv.PublishPosts)
	}
	for _, f := range srv.Versions[0] {
		if f.Path == ".env" || f.Path == "env.txt" {
			t.Fatalf("%s was published", f.Path)
		}
	}
}
