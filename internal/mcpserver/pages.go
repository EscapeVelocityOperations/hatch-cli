package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/pagepack"
	"github.com/mark3labs/mcp-go/mcp"
)

// Hatch pages (h-8iz8): publish_page, list_pages, list_page_versions,
// remove_page. Purge (forgetting a name) stays CLI-only.

// --- publish_page ---

func publishPageTool() mcp.Tool {
	return mcp.NewTool("publish_page",
		mcp.WithDescription("Publish an HTML page (or a folder with an index.html) on a protected egg at /pages/<name>/, "+
			"without a build or deploy. Publishing again at the same name creates the next version: the main link shows "+
			"the latest and /pages/<name>/v<N>/ keeps each version. Pass exactly one of path (local .html file or folder) "+
			"or html (inline page, at most 2 MB). The egg must be password- or email-protected."),
		mcp.WithString("app", mcp.Required(), mcp.Description("App slug (name)")),
		mcp.WithString("path", mcp.Description("Local .html file or folder containing index.html")),
		mcp.WithString("html", mcp.Description("Inline HTML published as the page's index.html")),
		mcp.WithString("at", mcp.Description("Page path or name, e.g. /pages/bilan/ (default: from title, else the file name)")),
		mcp.WithString("title", mcp.Description("Page title shown to visitors")),
		mcp.WithString("as", mcp.Description("Author label shown to visitors, e.g. \"Claude (bosco)\" (default: the token's name)")),
		mcp.WithString("date", mcp.Description("Display date YYYY-MM-DD (default: today)")),
	)
}

func publishPageHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return toolError("failed to publish page: missing required parameter 'app'")
	}
	path, html := req.GetString("path", ""), req.GetString("html", "")
	if (path == "") == (html == "") {
		return toolError("failed to publish page: pass exactly one of 'path' or 'html'")
	}
	at, title := req.GetString("at", ""), req.GetString("title", "")

	var pack *pagepack.Pack
	if html != "" {
		if at == "" && pagepack.Slugify(title) == "" {
			return toolError("failed to publish page: inline html needs 'at' or a 'title' to name the page")
		}
		pack, err = pagepack.FromHTML(html)
	} else {
		pack, err = pagepack.Build(path)
	}
	if err != nil {
		return toolError("failed to publish page: %v", err)
	}
	name := at
	if name == "" {
		name = pagepack.Slugify(title)
	}
	if name == "" && path != "" {
		base := filepath.Base(filepath.Clean(path))
		name = pagepack.Slugify(strings.TrimSuffix(base, filepath.Ext(base)))
	}
	if name, err = api.NormalizePageName(name); err != nil || name == "index" {
		return toolError("failed to publish page: cannot derive a page name, pass 'at' (e.g. /pages/bilan/)")
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to publish page: %v", err)
	}
	res, err := client.PublishPage(slug, pack.Archive, api.PagePublishMeta{
		Name: name, Title: title, AuthorLabel: req.GetString("as", ""), DisplayDate: req.GetString("date", ""),
	})
	if err != nil {
		return toolError("failed to publish page: %v", err)
	}
	text := fmt.Sprintf("Published %s v%d on %s (%d files, %d bytes, %d already stored).\nLink: %s\nVersion: %s",
		res.Name, res.Version, slug, res.Files, res.Bytes, res.DedupBytes, res.URL, res.VersionURL)
	if len(pack.Skipped) > 0 {
		text += "\nLeft out (hidden or not a regular file): " + strings.Join(pack.Skipped, ", ")
	}
	return mcp.NewToolResultText(text), nil
}

// --- list_pages ---

func listPagesTool() mcp.Tool {
	return mcp.NewTool("list_pages",
		mcp.WithDescription("List an egg's published pages with their latest version, as JSON {egg, served, pages}."),
		mcp.WithString("app", mcp.Required(), mcp.Description("App slug (name)")),
		mcp.WithReadOnlyHintAnnotation(true),
	)
}

func listPagesHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return toolError("failed to list pages: missing required parameter 'app'")
	}
	client, err := newClient()
	if err != nil {
		return toolError("failed to list pages: %v", err)
	}
	l, err := client.ListPages(slug)
	if err != nil {
		return toolError("failed to list pages: %v", err)
	}
	return jsonResult(map[string]any{"egg": slug, "served": l.Gateway, "pages": l.Pages})
}

// --- list_page_versions ---

func listPageVersionsTool() mcp.Tool {
	return mcp.NewTool("list_page_versions",
		mcp.WithDescription("List the versions of one page, newest first (number, date, author, link), as JSON."),
		mcp.WithString("app", mcp.Required(), mcp.Description("App slug (name)")),
		mcp.WithString("page", mcp.Required(), mcp.Description("Page name or path, e.g. bilan or /pages/bilan/")),
		mcp.WithReadOnlyHintAnnotation(true),
	)
}

func listPageVersionsHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return toolError("failed to list page versions: missing required parameter 'app'")
	}
	page, err := req.RequireString("page")
	if err != nil {
		return toolError("failed to list page versions: missing required parameter 'page'")
	}
	client, err := newClient()
	if err != nil {
		return toolError("failed to list page versions: %v", err)
	}
	v, err := client.ListPageVersions(slug, page)
	if err != nil {
		return toolError("failed to list page versions: %v", err)
	}
	return jsonResult(map[string]any{"egg": slug, "page": v})
}

// --- remove_page ---

func removePageTool() mcp.Tool {
	return mcp.NewTool("remove_page",
		mcp.WithDescription("Remove a published page and all its versions, or only one version. Version numbers are "+
			"never reused: republishing the name continues at the next number."),
		mcp.WithString("app", mcp.Required(), mcp.Description("App slug (name)")),
		mcp.WithString("page", mcp.Required(), mcp.Description("Page name or path, e.g. bilan or /pages/bilan/")),
		mcp.WithNumber("version", mcp.Description("Remove only this version number")),
		mcp.WithDestructiveHintAnnotation(true),
	)
}

func removePageHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return toolError("failed to remove page: missing required parameter 'app'")
	}
	page, err := req.RequireString("page")
	if err != nil {
		return toolError("failed to remove page: missing required parameter 'page'")
	}
	version := req.GetInt("version", 0)
	if version < 0 {
		return toolError("failed to remove page: version must be a positive number")
	}
	client, err := newClient()
	if err != nil {
		return toolError("failed to remove page: %v", err)
	}
	res, err := client.RemovePage(slug, page, version, false)
	if err != nil {
		return toolError("failed to remove page: %v", err)
	}
	switch {
	case version > 0 && res.PageRemoved:
		return mcp.NewToolResultText(fmt.Sprintf("Removed v%d of %s on %s; it was the last version, so the page is gone.", version, res.Name, slug)), nil
	case version > 0:
		return mcp.NewToolResultText(fmt.Sprintf("Removed v%d of %s on %s.", version, res.Name, slug)), nil
	default:
		return mcp.NewToolResultText(fmt.Sprintf("Removed page %s on %s.", res.Name, slug)), nil
	}
}

func jsonResult(v any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return toolError("encoding result: %v", err)
	}
	return mcp.NewToolResultText(string(b)), nil
}
