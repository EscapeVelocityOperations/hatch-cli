package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/sitedeploy"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/sitedeploy/sitetest"
)

// --- T-506: deploy_app on an app that serves a Hatch site ---

func siteServer(t *testing.T) (*sitetest.Server, string) {
	t.Helper()
	saveAndRestore(t)
	setAuthToken("tok")
	old := sitedeploy.RetryDelay
	sitedeploy.RetryDelay = 0
	t.Cleanup(func() { sitedeploy.RetryDelay = old })
	srv := sitetest.New(t, "site-a1b2")
	newAPIClient = func(token string) *api.Client { return api.NewTestClient(token, srv.URL) }

	dir := t.TempDir()
	for name, body := range map[string]string{"index.html": "<h1>hi</h1>", "app.js": "x()", ".env": "S=1"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return srv, dir
}

func deploySiteText(t *testing.T, args map[string]interface{}) (string, bool) {
	t.Helper()
	result, err := deployAppHandler(context.Background(), makeReq(args))
	if err != nil {
		t.Fatal(err)
	}
	return resultText(t, result), result.IsError
}

func TestDeployAppHandler_SitePublishesChangedFilesOnly(t *testing.T) {
	srv, dir := siteServer(t)
	srv.HasSite = true

	text, isErr := deploySiteText(t, deployReq(dir, nil))
	if isErr || !strings.Contains(text, "Hatch site: version 1 is live") || !strings.Contains(text, "uploaded 2") {
		t.Fatalf("result (error %v): %s", isErr, text)
	}
	if !strings.Contains(text, "env files are never sent") {
		t.Fatalf("no env note: %s", text)
	}
	if srv.ArtifactUploads != 0 || srv.PublishPosts != 1 {
		t.Fatalf("artifacts %d, publications %d", srv.ArtifactUploads, srv.PublishPosts)
	}

	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("y()"), 0o644); err != nil {
		t.Fatal(err)
	}
	text, isErr = deploySiteText(t, deployReq(dir, nil))
	if isErr || !strings.Contains(text, "version 2 is live") || !strings.Contains(text, "uploaded 1 ") {
		t.Fatalf("second deploy (error %v): %s", isErr, text)
	}
}

func TestDeployAppHandler_SiteFallsBackToArtifact(t *testing.T) {
	t.Run("app without site", func(t *testing.T) {
		srv, dir := siteServer(t)
		text, isErr := deploySiteText(t, deployReq(dir, nil))
		if isErr || srv.ArtifactUploads != 1 || srv.ManifestPosts != 0 || !strings.Contains(text, "Deployed successfully") {
			t.Fatalf("artifacts %d, manifests %d, result: %s", srv.ArtifactUploads, srv.ManifestPosts, text)
		}
	})
	t.Run("404 sites_disabled", func(t *testing.T) {
		srv, dir := siteServer(t)
		srv.HasSite = true
		srv.Disabled = true
		text, isErr := deploySiteText(t, deployReq(dir, nil))
		if isErr || srv.DisabledHits != 1 || srv.PublishPosts != 0 || srv.ArtifactUploads != 1 {
			t.Fatalf("sites_disabled %d, publications %d, artifacts %d, result: %s", srv.DisabledHits, srv.PublishPosts, srv.ArtifactUploads, text)
		}
	})
}

func TestDeployAppHandler_Site413(t *testing.T) {
	srv, dir := siteServer(t)
	srv.HasSite = true
	srv.MaxBytes = 5
	text, isErr := deploySiteText(t, deployReq(dir, nil))
	if !isErr || !strings.Contains(text, "413") || !strings.Contains(text, "site_too_large") {
		t.Fatalf("result (error %v): %s", isErr, text)
	}
	if srv.BlobPosts != 0 || srv.ArtifactUploads != 0 {
		t.Fatalf("blobs %d, artifacts %d", srv.BlobPosts, srv.ArtifactUploads)
	}
}
