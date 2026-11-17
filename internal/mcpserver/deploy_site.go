package mcpserver

import (
	"errors"
	"fmt"
	"strings"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/sitedeploy"
	"github.com/mark3labs/mcp-go/mcp"
)

// deploySiteApp is deploy_app with runtime static on an existing app that
// serves a Hatch site (T-506): the file list goes, then only the files the
// platform lacks, then the list is published as the next version. The
// signal is the app's deployment history ("kind":"site" rows, hatch-api
// T-505). handled is false when the app is new or not a site, or when the
// platform has sites switched off: deploy_app then uploads an artifact as
// before. Errors of resolution and authentication are left to that path,
// which reports them as it always did.
func deploySiteApp(appSlug, name, deployTarget string) (result *mcp.CallToolResult, handled bool) {
	res, err := resolveDeploySlug(appSlug, name, deployTarget)
	if err != nil || res.Slug == "" {
		return nil, false
	}
	client, err := newClient()
	if err != nil {
		return nil, false
	}
	isSite, err := client.HasSite(res.Slug)
	if err != nil {
		r, _ := toolError("failed to deploy app: checking whether %s serves a Hatch site: %v", res.Slug, err)
		return r, true
	}
	if !isSite {
		return nil, false
	}

	m, err := sitedeploy.Build(deployTarget)
	if err != nil {
		r, _ := toolError("failed to deploy app: listing site files: %v", err)
		return r, true
	}
	out, err := sitedeploy.Publish(client, res.Slug, m, nil)
	if errors.Is(err, sitedeploy.ErrSitesDisabled) {
		return nil, false
	}
	if err != nil {
		r, _ := toolError("failed to deploy app: %v", err)
		return r, true
	}

	url := strings.TrimSuffix(out.Publish.URL, "/")
	if url == "" {
		url = fmt.Sprintf("https://%s.nest.gethatch.eu", res.Slug)
	}
	note := envExcludedNote(m.Excluded)
	if len(m.Skipped) > 0 {
		note += "\nNot published (symlinks leaving deploy_target or pointing at a directory, special files): " + strings.Join(m.Skipped, ", ")
	}
	return mcp.NewToolResultText(fmt.Sprintf(
		"Deployed successfully!\nApp: %s\nURL: %s\nRuntime: static (Hatch site: version %d is live, no image built)\n"+
			"Files: %d, uploaded %d (%s sent)\nApp resolved from: %s%s",
		res.Slug, url, out.Publish.Version, len(m.Files), out.UploadedFiles, sitedeploy.FormatBytes(out.SentBytes),
		res.Provenance, note)), true
}
