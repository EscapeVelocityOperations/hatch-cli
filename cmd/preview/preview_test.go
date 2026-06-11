package preview

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
)

// fakeAPIClient records preview API calls.
type fakeAPIClient struct {
	previews []api.Preview
	listErr  error

	deletedParent string
	deletedPR     int
	deleteCalls   int
	deleteErr     error
}

func (f *fakeAPIClient) ListPreviews(parentSlug string) ([]api.Preview, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.previews, nil
}

func (f *fakeAPIClient) DeletePreview(parentSlug string, prNumber int) error {
	f.deleteCalls++
	f.deletedParent = parentSlug
	f.deletedPR = prNumber
	return f.deleteErr
}

func fakeDeps(f *fakeAPIClient, parent string) *Deps {
	return &Deps{
		GetToken:      func() (string, error) { return "tok123", nil },
		NewAPIClient:  func(token string) APIClient { return f },
		ResolveParent: func() (string, error) { return parent, nil },
	}
}

func captureOutput(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// --- T-011: hatch preview list / rm --------------------------------------

// The preview group exposes list and rm subcommands.
func TestNewCmd_HasListAndRmSubcommands(t *testing.T) {
	cmd := NewCmd()
	var hasList, hasRm bool
	for _, sub := range cmd.Commands() {
		switch sub.Name() {
		case "list":
			hasList = true
		case "rm":
			hasRm = true
		}
	}
	if !hasList {
		t.Error("preview command missing 'list' subcommand")
	}
	if !hasRm {
		t.Error("preview command missing 'rm' subcommand")
	}
}

// list prints one row per active preview with PR number, slug, URL and
// expiry visible.
func TestRunList_PrintsPreviewRows(t *testing.T) {
	f := &fakeAPIClient{previews: []api.Preview{
		{Slug: "myapp-pr-41", PRNumber: 41, URL: "https://myapp-pr-41.nest.gethatch.eu", Status: "running", ExpiresAt: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)},
		{Slug: "myapp-pr-42", PRNumber: 42, URL: "https://myapp-pr-42.nest.gethatch.eu", Status: "sleeping", ExpiresAt: time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)},
	}}
	deps = fakeDeps(f, "myapp")
	defer func() { deps = defaultDeps() }()

	var err error
	out := captureOutput(func() { err = runList(nil, nil) })

	if err != nil {
		t.Fatalf("runList error: %v", err)
	}
	for _, want := range []string{"41", "myapp-pr-41", "https://myapp-pr-41.nest.gethatch.eu", "42", "myapp-pr-42"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q, got:\n%s", want, out)
		}
	}
}

// list with no previews says so instead of printing an empty table.
func TestRunList_Empty(t *testing.T) {
	deps = fakeDeps(&fakeAPIClient{}, "myapp")
	defer func() { deps = defaultDeps() }()

	var err error
	out := captureOutput(func() { err = runList(nil, nil) })

	if err != nil {
		t.Fatalf("runList error: %v", err)
	}
	if !strings.Contains(strings.ToLower(out), "no previews") {
		t.Fatalf("empty list should say 'no previews', got:\n%s", out)
	}
}

// rm accepts pr-<n> and bare <n> refs and deletes the matching preview
// of the resolved parent.
func TestRunRm_DeletesByRef(t *testing.T) {
	for _, tt := range []struct {
		ref    string
		wantPR int
	}{
		{ref: "pr-42", wantPR: 42},
		{ref: "7", wantPR: 7},
	} {
		t.Run(tt.ref, func(t *testing.T) {
			f := &fakeAPIClient{}
			deps = fakeDeps(f, "myapp")
			defer func() { deps = defaultDeps() }()

			var err error
			captureOutput(func() { err = runRm(nil, []string{tt.ref}) })

			if err != nil {
				t.Fatalf("runRm error: %v", err)
			}
			if f.deleteCalls != 1 || f.deletedParent != "myapp" || f.deletedPR != tt.wantPR {
				t.Fatalf("DeletePreview called %d times with (%q, %d), want once with (\"myapp\", %d)",
					f.deleteCalls, f.deletedParent, f.deletedPR, tt.wantPR)
			}
		})
	}
}

// rm with a garbage ref errors mentioning the ref and never calls the API.
func TestRunRm_InvalidRef(t *testing.T) {
	f := &fakeAPIClient{}
	deps = fakeDeps(f, "myapp")
	defer func() { deps = defaultDeps() }()

	var err error
	captureOutput(func() { err = runRm(nil, []string{"abc"}) })

	if err == nil {
		t.Fatal("expected an error for ref \"abc\"")
	}
	if !strings.Contains(err.Error(), "abc") {
		t.Fatalf("error should name the bad ref, got: %v", err)
	}
	if f.deleteCalls != 0 {
		t.Fatal("invalid ref must not reach the API")
	}
}

// rm surfaces API failure (e.g. preview not found) to the user.
func TestRunRm_APIErrorSurfaced(t *testing.T) {
	f := &fakeAPIClient{deleteErr: errors.New("preview not found")}
	deps = fakeDeps(f, "myapp")
	defer func() { deps = defaultDeps() }()

	var err error
	captureOutput(func() { err = runRm(nil, []string{"pr-42"}) })

	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("API error must surface, got: %v", err)
	}
}

// Without a resolvable parent app (no .hatch.toml, no --app) both commands
// fail with the resolver's explanation.
func TestRunList_ParentResolutionFailure(t *testing.T) {
	deps = &Deps{
		GetToken:      func() (string, error) { return "tok123", nil },
		NewAPIClient:  func(token string) APIClient { return &fakeAPIClient{} },
		ResolveParent: func() (string, error) { return "", errors.New("no .hatch.toml found — run from your app directory or pass --app") },
	}
	defer func() { deps = defaultDeps() }()

	var err error
	captureOutput(func() { err = runList(nil, nil) })

	if err == nil || !strings.Contains(err.Error(), ".hatch.toml") {
		t.Fatalf("parent resolution failure must surface, got: %v", err)
	}
}
