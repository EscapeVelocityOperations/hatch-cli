package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testCommentID = "0f8c2b1e-3a4d-4e5f-8a9b-1c2d3e4f5a6b"

// commentServer records the last request and answers with status + body.
type commentServer struct {
	method, path, rawQuery, auth string
	body                         map[string]any
}

func newCommentServer(t *testing.T, status int, resp string) (*Client, *commentServer) {
	t.Helper()
	rec := &commentServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.method, rec.path, rec.rawQuery, rec.auth = r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization")
		rec.body = nil
		_ = json.NewDecoder(r.Body).Decode(&rec.body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	return NewTestClient("tok123", srv.URL), rec
}

func TestCommentSettings_Methods(t *testing.T) {
	for _, tc := range []struct {
		name   string
		call   func(c *Client) (*CommentSettings, error)
		method string
	}{
		{"get", func(c *Client) (*CommentSettings, error) { return c.GetCommentSettings("my-app") }, "GET"},
		{"enable", func(c *Client) (*CommentSettings, error) { return c.EnableComments("my-app") }, "PUT"},
		{"disable", func(c *Client) (*CommentSettings, error) { return c.DisableComments("my-app") }, "DELETE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := newCommentServer(t, 200, `{"comments_enabled":true,"email_protected":true,"comment_count":7}`)
			s, err := tc.call(c)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if rec.method != tc.method || rec.path != "/v1/apps/my-app/comments/settings" {
				t.Errorf("request = %s %s, want %s /v1/apps/my-app/comments/settings", rec.method, rec.path, tc.method)
			}
			if rec.auth != "Bearer tok123" {
				t.Errorf("auth = %q", rec.auth)
			}
			if !s.CommentsEnabled || !s.EmailProtected || s.CommentCount != 7 {
				t.Errorf("settings = %+v", s)
			}
		})
	}
}

func TestEnableComments_RequiresEmailProtection(t *testing.T) {
	c, _ := newCommentServer(t, 409, `{"error":"comments_require_email_protection"}`)
	_, err := c.EnableComments("my-app")
	if err == nil || !strings.Contains(err.Error(), "comments_require_email_protection") {
		t.Fatalf("err = %v, want the API's 409 code surfaced", err)
	}
}

func TestListComments_QueryAndDecode(t *testing.T) {
	c, rec := newCommentServer(t, 200, `{"threads":[{"id":"`+testCommentID+`","page":"/x/","parent_id":null,
		"anchor":{"exact":"hello","prefix":"a ","suffix":" b"},"body":"Typo here","author_email":"v@x.test",
		"author_kind":"visitor","author_label":null,"created_at":"2026-10-18T14:02:00Z","resolved_at":null,
		"resolved_by":null,"deleted_at":null,"deleted_by":null,"status":"open","replies":[]}],"next_cursor":"abc"}`)
	l, err := c.ListComments("my-app", ListCommentsOptions{Page: "/x/", Status: "all", Since: "2026-10-18", Limit: 10, Cursor: "zz"})
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if rec.method != "GET" || rec.path != "/v1/apps/my-app/comments" {
		t.Errorf("request = %s %s", rec.method, rec.path)
	}
	for _, want := range []string{"page=%2Fx%2F", "status=all", "since=2026-10-18", "limit=10", "cursor=zz"} {
		if !strings.Contains(rec.rawQuery, want) {
			t.Errorf("query %q missing %q", rec.rawQuery, want)
		}
	}
	if len(l.Threads) != 1 || l.Threads[0].Anchor == nil || l.Threads[0].Anchor.Exact != "hello" || l.Threads[0].Status != "open" {
		t.Errorf("threads = %+v", l.Threads)
	}
	if l.NextCursor == nil || *l.NextCursor != "abc" {
		t.Errorf("next_cursor = %v", l.NextCursor)
	}
}

func TestListComments_NoFiltersNoQuery(t *testing.T) {
	c, rec := newCommentServer(t, 200, `{"threads":[],"next_cursor":null}`)
	if _, err := c.ListComments("my-app", ListCommentsOptions{}); err != nil {
		t.Fatal(err)
	}
	if rec.rawQuery != "" {
		t.Errorf("query = %q, want none (API defaults)", rec.rawQuery)
	}
}

func TestListAllComments_FollowsCursor(t *testing.T) {
	var cursors []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := r.URL.Query().Get("cursor")
		cursors = append(cursors, cur)
		if r.URL.Query().Get("limit") != "200" {
			t.Errorf("limit = %q, want 200", r.URL.Query().Get("limit"))
		}
		switch cur {
		case "":
			_, _ = w.Write([]byte(`{"threads":[{"id":"a","replies":[]}],"next_cursor":"c1"}`))
		case "c1":
			_, _ = w.Write([]byte(`{"threads":[{"id":"b","replies":[]}],"next_cursor":null}`))
		default:
			t.Errorf("unexpected cursor %q", cur)
		}
	}))
	defer srv.Close()

	threads, err := NewTestClient("tok", srv.URL).ListAllComments("my-app", ListCommentsOptions{Status: "open"})
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 2 || threads[0].ID != "a" || threads[1].ID != "b" {
		t.Errorf("threads = %+v", threads)
	}
	if strings.Join(cursors, ",") != ",c1" {
		t.Errorf("cursors = %v", cursors)
	}
}

func TestGetCommentThread(t *testing.T) {
	c, rec := newCommentServer(t, 200, `{"id":"`+testCommentID+`","status":"resolved","replies":[{"id":"r1","body":"done"}]}`)
	th, err := c.GetCommentThread("my-app", testCommentID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.method != "GET" || rec.path != "/v1/apps/my-app/comments/"+testCommentID {
		t.Errorf("request = %s %s", rec.method, rec.path)
	}
	if th.Status != "resolved" || len(th.Replies) != 1 || th.Replies[0].Body != "done" {
		t.Errorf("thread = %+v", th)
	}
}

func TestCommentID_RejectsPathTricks(t *testing.T) {
	c, rec := newCommentServer(t, 200, `{}`)
	for _, bad := range []string{"", "../settings", "export", testCommentID + "/resolve", "abc"} {
		if _, err := c.GetCommentThread("my-app", bad); err == nil {
			t.Errorf("GetCommentThread(%q) accepted", bad)
		}
		if _, err := c.ResolveComment("my-app", bad); err == nil {
			t.Errorf("ResolveComment(%q) accepted", bad)
		}
		if bad == "" {
			continue // no parent = a page-level comment, a legitimate request
		}
		if _, err := c.CreateComment("my-app", NewComment{ParentID: bad, Body: "x"}); err == nil {
			t.Errorf("CreateComment(parent %q) accepted", bad)
		}
	}
	if rec.method != "" {
		t.Errorf("a request was sent: %s %s", rec.method, rec.path)
	}
}

func TestCreateComment_Reply(t *testing.T) {
	c, rec := newCommentServer(t, 201, `{"id":"r1","parent_id":"`+testCommentID+`","author_kind":"agent","author_label":"Claude"}`)
	out, err := c.CreateComment("my-app", NewComment{ParentID: testCommentID, Body: "Fixed.", AuthorLabel: "Claude"})
	if err != nil {
		t.Fatal(err)
	}
	if rec.method != "POST" || rec.path != "/v1/apps/my-app/comments" {
		t.Errorf("request = %s %s", rec.method, rec.path)
	}
	if rec.body["parent_id"] != testCommentID || rec.body["body"] != "Fixed." || rec.body["author_label"] != "Claude" {
		t.Errorf("body = %v", rec.body)
	}
	if _, ok := rec.body["page"]; ok {
		t.Errorf("empty page should be omitted: %v", rec.body)
	}
	if _, ok := rec.body["author_kind"]; ok {
		t.Errorf("empty author_kind should be omitted (API default agent): %v", rec.body)
	}
	if out.ID != "r1" || out.AuthorKind != "agent" {
		t.Errorf("comment = %+v", out)
	}
}

func TestCommentActions_Routes(t *testing.T) {
	for _, tc := range []struct {
		name         string
		call         func(c *Client) (*Comment, error)
		method, path string
	}{
		{"resolve", func(c *Client) (*Comment, error) { return c.ResolveComment("my-app", testCommentID) }, "POST", "/v1/apps/my-app/comments/" + testCommentID + "/resolve"},
		{"reopen", func(c *Client) (*Comment, error) { return c.ReopenComment("my-app", testCommentID) }, "POST", "/v1/apps/my-app/comments/" + testCommentID + "/reopen"},
		{"delete", func(c *Client) (*Comment, error) { return c.DeleteComment("my-app", testCommentID) }, "DELETE", "/v1/apps/my-app/comments/" + testCommentID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := newCommentServer(t, 200, `{"id":"`+testCommentID+`"}`)
			out, err := tc.call(c)
			if err != nil {
				t.Fatal(err)
			}
			if rec.method != tc.method || rec.path != tc.path {
				t.Errorf("request = %s %s, want %s %s", rec.method, rec.path, tc.method, tc.path)
			}
			if out.ID != testCommentID {
				t.Errorf("comment = %+v", out)
			}
		})
	}
}

func TestExportComments_Verbatim(t *testing.T) {
	const dump = `{"egg":"my-app","exported_at":"2026-10-19T08:00:00Z","comments":[{"id":"a"}]}`
	c, rec := newCommentServer(t, 200, dump)
	data, err := c.ExportComments("my-app")
	if err != nil {
		t.Fatal(err)
	}
	if rec.method != "GET" || rec.path != "/v1/apps/my-app/comments/export" {
		t.Errorf("request = %s %s", rec.method, rec.path)
	}
	if string(data) != dump {
		t.Errorf("export = %s", data)
	}
}

func TestExportComments_InvalidJSON(t *testing.T) {
	c, _ := newCommentServer(t, 200, `<html>`)
	if _, err := c.ExportComments("my-app"); err == nil {
		t.Fatal("expected an error for a non-JSON export")
	}
}

func TestPurgeComments(t *testing.T) {
	c, rec := newCommentServer(t, 200, `{"purged":12}`)
	n, err := c.PurgeComments("my-app", PurgeComments{AuthorEmail: "v@x.test", Before: "2026-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	if rec.method != "POST" || rec.path != "/v1/apps/my-app/comments/purge" {
		t.Errorf("request = %s %s", rec.method, rec.path)
	}
	if rec.body["author_email"] != "v@x.test" || rec.body["before"] != "2026-01-01" {
		t.Errorf("body = %v", rec.body)
	}
	if _, ok := rec.body["all"]; ok {
		t.Errorf("all=false should be omitted: %v", rec.body)
	}
	if n != 12 {
		t.Errorf("purged = %d", n)
	}
}

func TestComments_InvalidSlug(t *testing.T) {
	c, rec := newCommentServer(t, 200, `{}`)
	if _, err := c.ListComments("../x", ListCommentsOptions{}); err == nil {
		t.Error("ListComments accepted a bad slug")
	}
	if _, err := c.EnableComments("Bad Slug"); err == nil {
		t.Error("EnableComments accepted a bad slug")
	}
	if rec.method != "" {
		t.Errorf("a request was sent: %s %s", rec.method, rec.path)
	}
}
