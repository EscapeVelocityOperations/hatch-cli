package mcpserver

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/mark3labs/mcp-go/mcp"
)

var pageToolNames = []string{"publish_page", "list_pages", "list_page_versions", "remove_page"}

// h-ppn8 guard: NewServer() must expose the pages tools; remove_page is
// annotated destructive and purge stays CLI-only.
func TestNewServer_HasPageTools(t *testing.T) {
	s := NewServer()
	for _, name := range pageToolNames {
		if s.GetTool(name) == nil {
			t.Errorf("expected NewServer() to register a %q tool", name)
		}
	}
	if s.GetTool("purge_page") != nil {
		t.Error("purge_page must not be exposed over MCP")
	}
	rm := removePageTool()
	if rm.Annotations.DestructiveHint == nil || !*rm.Annotations.DestructiveHint {
		t.Error("remove_page must carry the destructive annotation")
	}
	if _, ok := rm.InputSchema.Properties["purge"]; ok {
		t.Error("remove_page must not offer purge")
	}
}

func TestPageHandlers_MissingParams(t *testing.T) {
	for _, h := range []func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error){
		publishPageHandler, listPagesHandler, listPageVersionsHandler, removePageHandler,
	} {
		result, err := h(context.Background(), makeReq(map[string]interface{}{}))
		assertError(t, result, err, "missing required parameter 'app'")
	}
	result, err := listPageVersionsHandler(context.Background(), makeReq(map[string]interface{}{"app": "e"}))
	assertError(t, result, err, "missing required parameter 'page'")
	result, err = publishPageHandler(context.Background(), makeReq(map[string]interface{}{"app": "e"}))
	assertError(t, result, err, "exactly one of 'path' or 'html'")
	result, err = publishPageHandler(context.Background(), makeReq(map[string]interface{}{"app": "e", "html": "<p>x</p>", "path": "x.html"}))
	assertError(t, result, err, "exactly one of 'path' or 'html'")
	result, err = publishPageHandler(context.Background(), makeReq(map[string]interface{}{"app": "e", "html": "<p>x</p>"}))
	assertError(t, result, err, "needs 'at' or a 'title'")
}

func TestPublishPage_InlineHTML(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	var meta api.PagePublishMeta
	var names []string
	newMockServer(t, map[string]http.HandlerFunc{
		"POST /v1/apps/e/pages": func(w http.ResponseWriter, r *http.Request) {
			_ = json.Unmarshal([]byte(r.Header.Get("X-Pages-Metadata")), &meta)
			zr, err := gzip.NewReader(r.Body)
			if err == nil {
				tr := tar.NewReader(zr)
				for h, err := tr.Next(); err == nil; h, err = tr.Next() {
					names = append(names, h.Name)
				}
			}
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(api.PagePublishResult{Name: "bilan", Version: 3, Files: 1, Bytes: 9,
				URL: "https://e.hatchapp.eu/pages/bilan/", VersionURL: "https://e.hatchapp.eu/pages/bilan/v3/"})
		},
	})
	result, err := publishPageHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "e", "html": "<p>x</p>", "title": "Bilan", "as": "Claude (bosco)",
	}))
	text := assertSuccess(t, result, err)
	if !strings.Contains(text, "Published bilan v3 on e") || !strings.Contains(text, "Version: https://e.hatchapp.eu/pages/bilan/v3/") {
		t.Errorf("text = %q", text)
	}
	if meta.Name != "bilan" || meta.AuthorLabel != "Claude (bosco)" || len(names) != 1 || names[0] != "index.html" {
		t.Errorf("meta=%+v entries=%v", meta, names)
	}
}

func TestPublishPage_PathAndAPIError(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"POST /v1/apps/e/pages": func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(409)
			_, _ = io.WriteString(w, `{"error":"pages_require_gateway"}`)
		},
	})
	p := filepath.Join(t.TempDir(), "rapport.html")
	_ = os.WriteFile(p, []byte("<p>r</p>"), 0o644)
	result, err := publishPageHandler(context.Background(), makeReq(map[string]interface{}{"app": "e", "path": p}))
	assertError(t, result, err, "pages_require_gateway")
}

func TestListAndRemovePage(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	var removeQuery string
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps/e/pages":       jsonHandler(api.PageList{Gateway: true, Pages: []api.PageSummary{{Name: "bilan", LatestVersion: 2}}}),
		"GET /v1/apps/e/pages/bilan": jsonHandler(api.PageVersions{Name: "bilan", LatestVersion: 2}),
		"DELETE /v1/apps/e/pages/bilan": func(w http.ResponseWriter, r *http.Request) {
			removeQuery = r.URL.RawQuery
			_ = json.NewEncoder(w).Encode(api.PageRemoveResult{Name: "bilan", RemovedVersion: 1})
		},
	})
	result, err := listPagesHandler(context.Background(), makeReq(map[string]interface{}{"app": "e"}))
	text := assertSuccess(t, result, err)
	if !strings.Contains(text, `"served": true`) || !strings.Contains(text, `"latest_version": 2`) {
		t.Errorf("list = %s", text)
	}
	result, err = listPageVersionsHandler(context.Background(), makeReq(map[string]interface{}{"app": "e", "page": "/pages/bilan/"}))
	text = assertSuccess(t, result, err)
	if !strings.Contains(text, `"name": "bilan"`) {
		t.Errorf("versions = %s", text)
	}
	result, err = removePageHandler(context.Background(), makeReq(map[string]interface{}{"app": "e", "page": "bilan", "version": float64(1)}))
	text = assertSuccess(t, result, err)
	if text != "Removed v1 of bilan on e." || removeQuery != "version=1" {
		t.Errorf("remove = %q query=%q", text, removeQuery)
	}
}
