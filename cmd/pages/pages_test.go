package pages

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/exitcode"
)

type mockClient struct {
	calls      []string
	slugs      []string
	lastMeta   api.PagePublishMeta
	lastArch   []byte
	lastRemove struct {
		page    string
		version int
		purge   bool
	}
	err  error
	list *api.PageList
}

func (m *mockClient) rec(name, slug string) {
	m.calls = append(m.calls, name)
	m.slugs = append(m.slugs, slug)
}

func (m *mockClient) PublishPage(slug string, archive []byte, meta api.PagePublishMeta) (*api.PagePublishResult, error) {
	m.rec("publish", slug)
	m.lastMeta, m.lastArch = meta, archive
	if m.err != nil {
		return nil, m.err
	}
	return &api.PagePublishResult{Name: meta.Name, Title: meta.Title, Version: 2, Files: 1, Bytes: 2048, DedupBytes: 1024,
		URL: "https://" + slug + ".hatchapp.eu/pages/" + meta.Name + "/", VersionURL: "https://" + slug + ".hatchapp.eu/pages/" + meta.Name + "/v2/"}, nil
}

func (m *mockClient) ListPages(slug string) (*api.PageList, error) {
	m.rec("list", slug)
	if m.err != nil {
		return nil, m.err
	}
	if m.list != nil {
		return m.list, nil
	}
	label := "Claude (bosco)"
	return &api.PageList{Gateway: true, Pages: []api.PageSummary{{Name: "bilan", Title: "Bilan\nQ3", LatestVersion: 2, Versions: 2,
		AuthorLabel: &label, DisplayDate: "2026-10-21", URL: "https://e/pages/bilan/"}}}, nil
}

func (m *mockClient) ListPageVersions(slug, page string) (*api.PageVersions, error) {
	m.rec("versions "+page, slug)
	if m.err != nil {
		return nil, m.err
	}
	return &api.PageVersions{Name: "bilan", Title: "Bilan", URL: "https://e/pages/bilan/", LatestVersion: 2, Versions: []api.PageVersion{
		{Version: 2, Latest: true, AuthorEmail: "a@b.c", DisplayDate: "2026-10-21", URL: "https://e/pages/bilan/v2/"},
		{Version: 1, AuthorEmail: "a@b.c", DisplayDate: "2026-10-20", URL: "https://e/pages/bilan/v1/"},
	}}, nil
}

func (m *mockClient) RemovePage(slug, page string, version int, purge bool) (*api.PageRemoveResult, error) {
	m.rec("remove "+page, slug)
	m.lastRemove.page, m.lastRemove.version, m.lastRemove.purge = page, version, purge
	if m.err != nil {
		return nil, m.err
	}
	return &api.PageRemoveResult{Name: page, RemovedVersion: version, PageRemoved: version == 0}, nil
}

type testEnv struct {
	tty      bool
	loggedIn bool
}

func withDeps(t *testing.T, mock *mockClient) *testEnv {
	t.Helper()
	env := &testEnv{loggedIn: true}
	deps = &Deps{
		GetToken: func() (string, error) {
			if !env.loggedIn {
				return "", nil
			}
			return "tok", nil
		},
		NewAPIClient:    func(string) APIClient { return mock },
		Now:             func() time.Time { return time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC) },
		StdinIsTerminal: func() bool { return env.tty },
	}
	t.Cleanup(func() { deps = defaultDeps() })
	return env
}

func run(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func htmlFile(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("<h1>x</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// h-abmr regression: the positional egg is the one acted on.
func TestPositionalSlugIsRespected(t *testing.T) {
	f := htmlFile(t, "rapport.html")
	for _, tc := range []struct {
		args []string
		call string
	}{
		{[]string{"publish", "other-egg", f}, "publish"},
		{[]string{"list", "other-egg"}, "list"},
		{[]string{"versions", "other-egg", "bilan"}, "versions bilan"},
		{[]string{"remove", "other-egg", "bilan", "--yes"}, "remove bilan"},
	} {
		mock := &mockClient{}
		withDeps(t, mock)
		if _, _, err := run(t, "", tc.args...); err != nil {
			t.Errorf("%v: %v", tc.args, err)
			continue
		}
		if len(mock.calls) != 1 || mock.calls[0] != tc.call || mock.slugs[0] != "other-egg" {
			t.Errorf("%v: calls=%v slugs=%v", tc.args, mock.calls, mock.slugs)
		}
	}
}

func TestPositionalArity(t *testing.T) {
	for _, args := range [][]string{
		{"other-egg"},
		{"publish", "e"},
		{"publish", "e", "a.html", "b.html"},
		{"list"},
		{"list", "a", "b"},
		{"versions", "e"},
		{"remove", "e"},
		{"remove", "e", "p", "x", "--yes"},
	} {
		mock := &mockClient{}
		withDeps(t, mock)
		if _, _, err := run(t, "", args...); err == nil {
			t.Errorf("%v: accepted", args)
		}
		if len(mock.calls) != 0 {
			t.Errorf("%v: API called %v", args, mock.calls)
		}
	}
}

func TestPublish_TextOutputAndName(t *testing.T) {
	mock := &mockClient{}
	withDeps(t, mock)
	out, _, err := run(t, "", "publish", "portail-jbi", htmlFile(t, "rapport.html"), "--at", "/pages/bilan/",
		"--title", "Bilan", "--as", "Claude (bosco)", "--date", "2026-10-20")
	if err != nil {
		t.Fatal(err)
	}
	want := "Published bilan v2 on portail-jbi (1 files, 2.0 KB, 1.0 KB already stored)\n" +
		"  Link:    https://portail-jbi.hatchapp.eu/pages/bilan/\n" +
		"  Version: https://portail-jbi.hatchapp.eu/pages/bilan/v2/\n"
	if out != want {
		t.Fatalf("output:\n%s\nwant:\n%s", out, want)
	}
	if mock.lastMeta != (api.PagePublishMeta{Name: "bilan", Title: "Bilan", AuthorLabel: "Claude (bosco)", DisplayDate: "2026-10-20"}) {
		t.Fatalf("meta = %+v", mock.lastMeta)
	}
	if len(mock.lastArch) == 0 {
		t.Fatal("no archive sent")
	}

	for _, tc := range []struct {
		args []string
		name string
	}{
		{[]string{"--title", "Bilan d'été"}, "bilan-d-ete"},
		{nil, "rapport"},
		{[]string{"--owner"}, "rapport"},
	} {
		mock = &mockClient{}
		withDeps(t, mock)
		args := append([]string{"publish", "e", htmlFile(t, "rapport.html")}, tc.args...)
		if _, _, err := run(t, "", args...); err != nil || mock.lastMeta.Name != tc.name {
			t.Errorf("%v: name=%q err=%v", tc.args, mock.lastMeta.Name, err)
		}
		if len(tc.args) == 1 && tc.args[0] == "--owner" && mock.lastMeta.AuthorKind != "owner" {
			t.Errorf("--owner: kind=%q", mock.lastMeta.AuthorKind)
		}
	}
}

func TestPublish_JSON(t *testing.T) {
	mock := &mockClient{}
	withDeps(t, mock)
	out, _, err := run(t, "", "publish", "e", htmlFile(t, "x.html"), "--at", "bilan", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	for _, k := range []string{"egg", "name", "title", "version", "url", "version_url", "files", "bytes", "dedup_bytes", "skipped"} {
		if _, ok := m[k]; !ok {
			t.Errorf("json lacks %q: %s", k, out)
		}
	}
	if m["version"] != float64(2) || m["egg"] != "e" {
		t.Fatalf("json = %v", m)
	}
}

// h-59ej: every failure carries exit status 1, terminal or not.
func TestErrorsExitNonZeroWithHints(t *testing.T) {
	mock := &mockClient{err: errors.New(`API error 409: {"error":"pages_require_gateway"}`)}
	withDeps(t, mock)
	_, _, err := run(t, "", "publish", "open-egg", htmlFile(t, "x.html"))
	if code, ok := exitcode.From(err); !ok || code != 1 {
		t.Fatalf("exit code = %d %v (err %v)", code, ok, err)
	}
	if !strings.Contains(err.Error(), "must be protected first") {
		t.Fatalf("hint missing: %v", err)
	}
	for _, args := range [][]string{
		{"publish", "e", "/does/not/exist.html"},
		{"publish", "e", htmlFile(t, "x.html"), "--date", "21/10/2026"},
		{"publish", "e", htmlFile(t, "x.html"), "--at", "Bad Name"},
		{"list", "e"},
	} {
		withDeps(t, mock)
		_, _, err := run(t, "", args...)
		if code, ok := exitcode.From(err); !ok || code != 1 {
			t.Errorf("%v: exit code = %d %v (err %v)", args, code, ok, err)
		}
	}
	env := withDeps(t, &mockClient{})
	env.loggedIn = false
	if _, _, err := run(t, "", "list", "e"); err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("logged out: %v", err)
	}
}

// h-8iz8 REWORK #3: pages switched off server-side is a 404 that must not
// read as a missing egg.
func TestPagesDisabledHint(t *testing.T) {
	withDeps(t, &mockClient{err: errors.New(`API error 404: {"error":"pages_disabled","message":"Hatch pages are not available on this platform yet"}`)})
	_, _, err := run(t, "", "list", "e")
	if err == nil || !strings.Contains(err.Error(), "not open on this platform yet") || strings.Contains(err.Error(), "no such egg") {
		t.Fatalf("pages disabled: %v", err)
	}
}

func TestRemove_ConfirmationRules(t *testing.T) {
	mock := &mockClient{}
	withDeps(t, mock)
	_, _, err := run(t, "", "remove", "e", "bilan")
	if err == nil || !strings.Contains(err.Error(), "without --yes outside a terminal") || len(mock.calls) != 0 {
		t.Fatalf("non-tty without --yes: %v calls=%v", err, mock.calls)
	}
	if code, _ := exitcode.From(err); code != 1 {
		t.Fatalf("refusal exit code = %d", code)
	}

	env := withDeps(t, mock)
	env.tty = true
	if _, _, err := run(t, "n\n", "remove", "e", "bilan"); err == nil || len(mock.calls) != 0 {
		t.Fatalf("declined: %v calls=%v", err, mock.calls)
	}
	out, _, err := run(t, "y\n", "remove", "e", "bilan", "--version", "1")
	if err != nil || mock.lastRemove.version != 1 || mock.lastRemove.purge || !strings.Contains(out, "Removed v1 of bilan on e.") {
		t.Fatalf("confirmed: %v %+v %q", err, mock.lastRemove, out)
	}
	if _, _, err := run(t, "", "remove", "e", "/pages/bilan/", "--purge", "--yes"); err != nil || !mock.lastRemove.purge || mock.lastRemove.page != "bilan" {
		t.Fatalf("purge: %v %+v", err, mock.lastRemove)
	}
	if _, _, err := run(t, "", "remove", "e", "bilan", "--purge", "--version", "2", "--yes"); err == nil {
		t.Fatal("--purge with --version accepted")
	}
}

func TestListAndVersionsOutput(t *testing.T) {
	mock := &mockClient{}
	withDeps(t, mock)
	out, _, err := run(t, "", "list", "e")
	if err != nil || !strings.Contains(out, "bilan") || !strings.Contains(out, "v2") || !strings.Contains(out, "Bilan Q3") ||
		!strings.Contains(out, "Claude (bosco)") {
		t.Fatalf("list: %v\n%s", err, out)
	}
	mock.list = &api.PageList{Gateway: false}
	out, errOut, _ := run(t, "", "list", "e")
	if !strings.Contains(out, "No pages on e.") || !strings.Contains(errOut, "not protected") {
		t.Fatalf("empty unprotected list: %q %q", out, errOut)
	}
	out, _, err = run(t, "", "versions", "e", "bilan")
	if err != nil || !strings.Contains(out, "v2 (latest)") || !strings.Contains(out, "https://e/pages/bilan/v1/") {
		t.Fatalf("versions: %v\n%s", err, out)
	}
	out, _, _ = run(t, "", "versions", "e", "bilan", "--json")
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil || m["egg"] != "e" {
		t.Fatalf("versions json: %v %s", err, out)
	}
}
