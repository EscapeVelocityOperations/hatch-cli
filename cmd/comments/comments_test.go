package comments

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/commentfmt"
)

const cid = "0f8c2b1e-3a4d-4e5f-8a9b-1c2d3e4f5a6b"

// mockClient records calls (slug + arguments) and returns canned results.
type mockClient struct {
	calls []string
	slugs []string

	settings  *api.CommentSettings
	threads   []api.CommentThread
	thread    *api.CommentThread
	created   *api.Comment
	export    []byte
	purged    int64
	err       error
	lastList  api.ListCommentsOptions
	lastNew   api.NewComment
	lastPurge api.PurgeComments
}

func (m *mockClient) rec(name, slug string) {
	m.calls = append(m.calls, name)
	m.slugs = append(m.slugs, slug)
}

func (m *mockClient) settingsOr() (*api.CommentSettings, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.settings == nil {
		return &api.CommentSettings{}, nil
	}
	return m.settings, nil
}

func (m *mockClient) GetCommentSettings(slug string) (*api.CommentSettings, error) {
	m.rec("status", slug)
	return m.settingsOr()
}

func (m *mockClient) EnableComments(slug string) (*api.CommentSettings, error) {
	m.rec("enable", slug)
	return m.settingsOr()
}

func (m *mockClient) DisableComments(slug string) (*api.CommentSettings, error) {
	m.rec("disable", slug)
	return m.settingsOr()
}

func (m *mockClient) ListAllComments(slug string, opts api.ListCommentsOptions) ([]api.CommentThread, error) {
	m.rec("list", slug)
	m.lastList = opts
	return m.threads, m.err
}

func (m *mockClient) GetCommentThread(slug, id string) (*api.CommentThread, error) {
	m.rec("show "+id, slug)
	return m.thread, m.err
}

func (m *mockClient) CreateComment(slug string, nc api.NewComment) (*api.Comment, error) {
	m.rec("create", slug)
	m.lastNew = nc
	if m.err != nil {
		return nil, m.err
	}
	return m.created, nil
}

func (m *mockClient) action(name, slug, id string) (*api.Comment, error) {
	m.rec(name+" "+id, slug)
	if m.err != nil {
		return nil, m.err
	}
	return &api.Comment{ID: id}, nil
}

func (m *mockClient) ResolveComment(slug, id string) (*api.Comment, error) {
	return m.action("resolve", slug, id)
}

func (m *mockClient) ReopenComment(slug, id string) (*api.Comment, error) {
	return m.action("reopen", slug, id)
}

func (m *mockClient) DeleteComment(slug, id string) (*api.Comment, error) {
	return m.action("delete", slug, id)
}

func (m *mockClient) ExportComments(slug string) ([]byte, error) {
	m.rec("export", slug)
	return m.export, m.err
}

func (m *mockClient) PurgeComments(slug string, p api.PurgeComments) (int64, error) {
	m.rec("purge", slug)
	m.lastPurge = p
	return m.purged, m.err
}

type testEnv struct {
	mock     *mockClient
	tty      bool
	written  map[string][]byte
	perms    map[string]os.FileMode
	loggedIn bool
}

func withDeps(t *testing.T, mock *mockClient) *testEnv {
	t.Helper()
	env := &testEnv{mock: mock, written: map[string][]byte{}, perms: map[string]os.FileMode{}, loggedIn: true}
	deps = &Deps{
		GetToken: func() (string, error) {
			if !env.loggedIn {
				return "", nil
			}
			return "tok123", nil
		},
		NewAPIClient:    func(token string) APIClient { return mock },
		Now:             func() time.Time { return time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC) },
		StdinIsTerminal: func() bool { return env.tty },
		WriteFile: func(name string, data []byte, perm os.FileMode) error {
			env.written[name], env.perms[name] = data, perm
			return nil
		},
	}
	t.Cleanup(func() { deps = defaultDeps() })
	return env
}

// run executes `hatch comments <args>` and returns stdout, stderr and the error.
func run(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	cmd.SilenceUsage = true
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// h-abmr regression: the positional slug is what gets mutated, never a cwd
// egg; extra or missing positionals are errors, never silently ignored.
func TestPositionalSlugIsRespected(t *testing.T) {
	for _, tc := range []struct {
		args []string
		call string
	}{
		{[]string{"enable", "other-egg"}, "enable"},
		{[]string{"disable", "other-egg"}, "disable"},
		{[]string{"status", "other-egg"}, "status"},
		{[]string{"list", "other-egg"}, "list"},
		{[]string{"show", "other-egg", cid}, "show " + cid},
		{[]string{"reply", "other-egg", cid, "hi"}, "create"},
		{[]string{"resolve", "other-egg", cid}, "resolve " + cid},
		{[]string{"reopen", "other-egg", cid}, "reopen " + cid},
		{[]string{"delete", "other-egg", cid}, "delete " + cid},
		{[]string{"export", "other-egg"}, "export"},
		{[]string{"purge", "other-egg", "--all", "--yes"}, "purge"},
	} {
		mock := &mockClient{thread: &api.CommentThread{}, created: &api.Comment{ID: "r1"}, export: []byte(`{}`)}
		withDeps(t, mock)
		if _, _, err := run(t, "", tc.args...); err != nil {
			t.Errorf("%v: %v", tc.args, err)
			continue
		}
		if len(mock.calls) != 1 || mock.calls[0] != tc.call || mock.slugs[0] != "other-egg" {
			t.Errorf("%v: calls=%v slugs=%v, want one %q on other-egg", tc.args, mock.calls, mock.slugs, tc.call)
		}
	}
}

func TestPositionalArity(t *testing.T) {
	for _, args := range [][]string{
		{"other-egg"},                  // group takes no positionals
		{"enable"},                     // missing egg
		{"enable", "a", "b"},           // extra positional
		{"list", "a", "b"},             // extra positional
		{"show", "a"},                  // missing id
		{"reply", "a", cid},            // missing text
		{"reply", "a", cid, "x", "y"},  // unquoted multi-word text
		{"resolve", "a", cid, "extra"}, // extra positional
		{"purge", "a"},                 // no scope flag
		{"purge", "a", "--all", "--author", "x@y.test", "--yes"},
		{"list", "a", "--open", "--all"},
		{"reply", "a", cid, "x", "--as", "Bot", "--owner"},
	} {
		mock := &mockClient{}
		withDeps(t, mock)
		if _, _, err := run(t, "", args...); err == nil {
			t.Errorf("%v: expected an error", args)
		}
		if len(mock.calls) != 0 {
			t.Errorf("%v: API called: %v", args, mock.calls)
		}
	}
}

func TestInvalidCommentIDRejectedLocally(t *testing.T) {
	mock := &mockClient{}
	withDeps(t, mock)
	for _, verb := range []string{"show", "resolve", "reopen", "delete"} {
		if _, _, err := run(t, "", verb, "my-egg", "../settings"); err == nil || !strings.Contains(err.Error(), "invalid comment id") {
			t.Errorf("%s: err = %v", verb, err)
		}
	}
	if _, _, err := run(t, "", "reply", "my-egg", "nope", "hi"); err == nil {
		t.Error("reply accepted a bad id")
	}
	if len(mock.calls) != 0 {
		t.Errorf("API called: %v", mock.calls)
	}
}

func TestNotLoggedIn(t *testing.T) {
	env := withDeps(t, &mockClient{})
	env.loggedIn = false
	if _, _, err := run(t, "", "list", "my-egg"); err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Errorf("err = %v", err)
	}
}

func TestEnable_Output(t *testing.T) {
	withDeps(t, &mockClient{settings: &api.CommentSettings{CommentsEnabled: true, EmailProtected: true, CommentCount: 3}})
	out, _, err := run(t, "", "enable", "portail-jbi")
	if err != nil {
		t.Fatal(err)
	}
	if out != "Comments are enabled on portail-jbi (3 stored).\n" {
		t.Errorf("out = %q", out)
	}
}

func TestEnable_RequiresEmailProtectionHint(t *testing.T) {
	withDeps(t, &mockClient{err: errors.New(`API error 409: {"error":"comments_require_email_protection"}`)})
	_, _, err := run(t, "", "enable", "portail-jbi")
	if err == nil || !strings.Contains(err.Error(), "hatch protect email enable") {
		t.Errorf("err = %v, want a hint to enable email protection", err)
	}
}

func TestStatus_JSONAndUnprotected(t *testing.T) {
	withDeps(t, &mockClient{settings: &api.CommentSettings{CommentCount: 2}})
	out, _, err := run(t, "", "status", "e", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if out != "{\n  \"comments_enabled\": false,\n  \"email_protected\": false,\n  \"comment_count\": 2\n}\n" {
		t.Errorf("json = %q", out)
	}
	out, _, _ = run(t, "", "status", "e")
	if !strings.Contains(out, "Comments are disabled on e (2 stored).") || !strings.Contains(out, "not under email protection") {
		t.Errorf("out = %q", out)
	}
}

func listFixture() []api.CommentThread {
	return []api.CommentThread{{
		Comment: api.Comment{ID: cid, Page: "/", Body: "Typo", AuthorEmail: "v@x.test", AuthorKind: "visitor",
			CreatedAt: time.Date(2026, 10, 18, 14, 2, 0, 0, time.UTC)},
		Status: "open", Replies: []api.Comment{},
	}}
}

func TestList_DefaultsAndMarkdown(t *testing.T) {
	mock := &mockClient{threads: listFixture()}
	withDeps(t, mock)
	out, _, err := run(t, "", "list", "portail-jbi")
	if err != nil {
		t.Fatal(err)
	}
	if mock.lastList != (api.ListCommentsOptions{Status: "open"}) {
		t.Errorf("opts = %+v, want status open only", mock.lastList)
	}
	if want := commentfmt.ThreadsMarkdown("portail-jbi", "open", listFixture()); out != want {
		t.Errorf("out = %q\nwant %q", out, want)
	}
}

func TestList_FiltersAndJSON(t *testing.T) {
	mock := &mockClient{threads: listFixture()}
	withDeps(t, mock)
	out, _, err := run(t, "", "list", "portail-jbi", "--resolved", "--page", "/pricing/", "--since", "24h", "--json")
	if err != nil {
		t.Fatal(err)
	}
	want := api.ListCommentsOptions{Status: "resolved", Page: "/pricing/", Since: "2026-10-18T08:00:00Z"}
	if mock.lastList != want {
		t.Errorf("opts = %+v, want %+v", mock.lastList, want)
	}
	js, _ := commentfmt.ThreadsAsJSON("portail-jbi", "resolved", listFixture())
	if out != string(js)+"\n" {
		t.Errorf("out = %s", out)
	}
	for _, s := range []string{"all", "deleted", "open"} {
		if _, _, err := run(t, "", "list", "e", "--"+s); err != nil || mock.lastList.Status != s {
			t.Errorf("--%s: status=%q err=%v", s, mock.lastList.Status, err)
		}
	}
}

func TestList_BadSince(t *testing.T) {
	mock := &mockClient{}
	withDeps(t, mock)
	if _, _, err := run(t, "", "list", "e", "--since", "yesterday"); err == nil {
		t.Fatal("expected an error")
	}
	if len(mock.calls) != 0 {
		t.Errorf("API called: %v", mock.calls)
	}
}

func TestShow_MarkdownAndJSON(t *testing.T) {
	th := listFixture()[0]
	withDeps(t, &mockClient{thread: &th})
	out, _, err := run(t, "", "show", "e", cid)
	if err != nil || out != commentfmt.ThreadMarkdown("e", th) {
		t.Errorf("out = %q err = %v", out, err)
	}
	out, _, err = run(t, "", "show", "e", cid, "--json")
	js, _ := commentfmt.ThreadAsJSON("e", th)
	if err != nil || out != string(js)+"\n" {
		t.Errorf("json = %q err = %v", out, err)
	}
}

func TestReply_AgentLabel(t *testing.T) {
	parent := cid
	mock := &mockClient{created: &api.Comment{ID: "r1", ParentID: &parent, AuthorKind: "agent", AuthorLabel: ptr("Claude (bosco)")}}
	withDeps(t, mock)
	out, _, err := run(t, "", "reply", "portail-jbi", cid, "Fixed, thanks!", "--as", "Claude (bosco)")
	if err != nil {
		t.Fatal(err)
	}
	if mock.lastNew != (api.NewComment{ParentID: cid, Body: "Fixed, thanks!", AuthorLabel: "Claude (bosco)"}) {
		t.Errorf("new = %+v", mock.lastNew)
	}
	if out != "Replied to thread "+cid+" on portail-jbi as Agent · Claude (bosco) (comment r1).\n" {
		t.Errorf("out = %q", out)
	}
}

func TestReply_OwnerAndStdin(t *testing.T) {
	mock := &mockClient{created: &api.Comment{ID: "r2", AuthorKind: "owner", AuthorEmail: "o@x.test"}}
	withDeps(t, mock)
	out, _, err := run(t, "line 1\nline 2\n", "reply", "e", cid, "-", "--owner")
	if err != nil {
		t.Fatal(err)
	}
	if mock.lastNew != (api.NewComment{ParentID: cid, Body: "line 1\nline 2\n", AuthorKind: "owner"}) {
		t.Errorf("new = %+v", mock.lastNew)
	}
	if !strings.Contains(out, "as o@x.test (owner)") {
		t.Errorf("out = %q", out)
	}
}

func TestReply_EmptyOrTooLong(t *testing.T) {
	mock := &mockClient{}
	withDeps(t, mock)
	if _, _, err := run(t, "  \n", "reply", "e", cid, "-"); err == nil {
		t.Error("empty stdin reply accepted")
	}
	if _, _, err := run(t, strings.Repeat("x", maxReplyStdin+1), "reply", "e", cid, "-"); err == nil {
		t.Error("oversized stdin reply accepted")
	}
	if len(mock.calls) != 0 {
		t.Errorf("API called: %v", mock.calls)
	}
}

func TestReply_CommentsDisabledHint(t *testing.T) {
	withDeps(t, &mockClient{err: errors.New(`API error 409: {"error":"comments_disabled"}`)})
	_, _, err := run(t, "", "reply", "e", cid, "hi")
	if err == nil || !strings.Contains(err.Error(), "hatch comments enable e") {
		t.Errorf("err = %v", err)
	}
}

func TestThreadActions_Output(t *testing.T) {
	for verb, want := range map[string]string{
		"resolve": "Resolved thread " + cid + " on e.\n",
		"reopen":  "Reopened thread " + cid + " on e.\n",
		"delete":  "Deleted comment " + cid + " on e.\n",
	} {
		withDeps(t, &mockClient{})
		out, _, err := run(t, "", verb, "e", cid)
		if err != nil || out != want {
			t.Errorf("%s: out=%q err=%v", verb, out, err)
		}
	}
}

func TestExport_StdoutAndFile(t *testing.T) {
	env := withDeps(t, &mockClient{export: []byte(`{"egg":"e","comments":[]}`)})
	out, _, err := run(t, "", "export", "e")
	if err != nil || out != "{\"egg\":\"e\",\"comments\":[]}\n" {
		t.Errorf("out=%q err=%v", out, err)
	}
	out, errOut, err := run(t, "", "export", "e", "-o", "dump.json")
	if err != nil || out != "" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if string(env.written["dump.json"]) != "{\"egg\":\"e\",\"comments\":[]}\n" || env.perms["dump.json"] != 0o600 {
		t.Errorf("written=%q perm=%o", env.written["dump.json"], env.perms["dump.json"])
	}
	if !strings.Contains(errOut, "dump.json") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestPurge_RequiresYesWithoutTerminal(t *testing.T) {
	mock := &mockClient{}
	env := withDeps(t, mock)
	env.tty = false
	_, _, err := run(t, "portail-jbi\n", "purge", "portail-jbi", "--all")
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("err = %v", err)
	}
	if len(mock.calls) != 0 {
		t.Errorf("API called: %v", mock.calls)
	}
}

func TestPurge_InteractiveConfirmation(t *testing.T) {
	mock := &mockClient{purged: 4}
	env := withDeps(t, mock)
	env.tty = true

	_, errOut, err := run(t, "wrong\n", "purge", "portail-jbi", "--author", "V@X.test")
	if err != nil || len(mock.calls) != 0 || !strings.Contains(errOut, "Cancelled") {
		t.Errorf("mismatch: err=%v calls=%v stderr=%q", err, mock.calls, errOut)
	}
	if !strings.Contains(errOut, "the comments by v@x.test on portail-jbi") {
		t.Errorf("prompt = %q", errOut)
	}

	out, _, err := run(t, "portail-jbi\n", "purge", "portail-jbi", "--author", "V@X.test", "--before", "30d")
	if err != nil {
		t.Fatal(err)
	}
	if mock.lastPurge != (api.PurgeComments{AuthorEmail: "v@x.test", Before: "2026-09-19T08:00:00Z"}) {
		t.Errorf("purge = %+v", mock.lastPurge)
	}
	if out != "Purged 4 comment(s) on portail-jbi.\n" {
		t.Errorf("out = %q", out)
	}
}

func TestPurge_YesAllAndBadBefore(t *testing.T) {
	mock := &mockClient{purged: 9}
	withDeps(t, mock)
	if _, _, err := run(t, "", "purge", "e", "--all", "-y"); err != nil || mock.lastPurge != (api.PurgeComments{All: true}) {
		t.Errorf("err=%v purge=%+v", err, mock.lastPurge)
	}
	mock.calls = nil
	_, _, err := run(t, "", "purge", "e", "--before", "soon", "--yes")
	if err == nil || !strings.Contains(err.Error(), "invalid before") {
		t.Errorf("err = %v", err)
	}
	if len(mock.calls) != 0 {
		t.Errorf("API called: %v", mock.calls)
	}
}

func ptr[T any](v T) *T { return &v }
