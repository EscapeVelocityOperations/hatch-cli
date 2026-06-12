package webhook

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockAPIClient implements APIClient (cmd/deploy fake-API pattern).
type mockAPIClient struct {
	createFn func(slug, url string, events []string) (*Webhook, string, error)
	listFn   func(slug string) ([]Webhook, error)
	deleteFn func(slug, id string) error
	testFn   func(slug, id string) error

	deletedID string
	testedID  string
}

func (m *mockAPIClient) CreateWebhook(slug, url string, events []string) (*Webhook, string, error) {
	if m.createFn != nil {
		return m.createFn(slug, url, events)
	}
	return &Webhook{ID: "wh-1", URL: url, Events: []string{"deploy"}, Active: true},
		"whsec_test_secret_abc123", nil
}

func (m *mockAPIClient) ListWebhooks(slug string) ([]Webhook, error) {
	if m.listFn != nil {
		return m.listFn(slug)
	}
	return nil, nil
}

func (m *mockAPIClient) DeleteWebhook(slug, id string) error {
	m.deletedID = id
	if m.deleteFn != nil {
		return m.deleteFn(slug, id)
	}
	return nil
}

func (m *mockAPIClient) TestWebhook(slug, id string) error {
	m.testedID = id
	if m.testFn != nil {
		return m.testFn(slug, id)
	}
	return nil
}

// withTestDeps wires deps to a logged-in user inside a tmp app dir whose
// .hatch.toml resolves to slug my-app, restoring the defaults afterwards.
func withTestDeps(t *testing.T, mock *mockAPIClient) {
	t.Helper()
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, ".hatch.toml"),
		[]byte("slug = \"my-app\"\nname = \"my-app\"\n"), 0o644); err != nil {
		t.Fatalf("setup .hatch.toml: %v", err)
	}
	deps = &Deps{
		GetToken:     func() (string, error) { return "tok123", nil },
		GetCwd:       func() (string, error) { return tmp, nil },
		NewAPIClient: func(token string) APIClient { return mock },
	}
	t.Cleanup(func() { deps = defaultDeps() })
}

func captureOutput(fn func() error) (string, error) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := fn()

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String(), err
}

// TestWebhookAdd_PrintsSecretWithShownOnceWarning: `hatch webhook add <url>`
// prints the plaintext secret exactly once, with a warning that it will not
// be shown again (spec h-xv5s7).
func TestWebhookAdd_PrintsSecretWithShownOnceWarning(t *testing.T) {
	mock := &mockAPIClient{}
	withTestDeps(t, mock)

	out, err := captureOutput(func() error {
		return runAdd(addCmd, []string{"https://93.184.216.34/hook"})
	})
	if err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	if !strings.Contains(out, "whsec_test_secret_abc123") {
		t.Errorf("add output must print the signing secret, got:\n%s", out)
	}
	lower := strings.ToLower(out)
	if !strings.Contains(lower, "once") {
		t.Errorf("add output must warn the secret is shown only once, got:\n%s", out)
	}
}

// TestWebhookAdd_NotLoggedIn: add without a token fails with the standard
// login hint.
func TestWebhookAdd_NotLoggedIn(t *testing.T) {
	withTestDeps(t, &mockAPIClient{})
	deps.GetToken = func() (string, error) { return "", nil }

	_, err := captureOutput(func() error {
		return runAdd(addCmd, []string{"https://93.184.216.34/hook"})
	})
	if err == nil {
		t.Fatal("expected error for unauthenticated user")
	}
	if !strings.Contains(err.Error(), "not logged in") {
		t.Errorf("error = %q, want the standard not-logged-in hint", err.Error())
	}
}

// TestWebhookList_RendersTableWithoutSecrets: `hatch webhook list` shows the
// registered webhooks (id + url) and never any secret material.
func TestWebhookList_RendersTableWithoutSecrets(t *testing.T) {
	mock := &mockAPIClient{listFn: func(slug string) ([]Webhook, error) {
		if slug != "my-app" {
			t.Errorf("list called with slug %q, want my-app (.hatch.toml)", slug)
		}
		return []Webhook{
			{ID: "wh-1", URL: "https://93.184.216.34/hook", Events: []string{"deploy"}, Active: true, LastStatus: "ok"},
			{ID: "wh-2", URL: "https://93.184.216.34/hook2", Events: []string{"deploy"}, Active: true},
		}, nil
	}}
	withTestDeps(t, mock)

	out, err := captureOutput(func() error { return runList(listCmd, nil) })
	if err != nil {
		t.Fatalf("runList: %v", err)
	}
	for _, want := range []string{"wh-1", "wh-2", "https://93.184.216.34/hook"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q, got:\n%s", want, out)
		}
	}
	if strings.Contains(strings.ToLower(out), "whsec") || strings.Contains(out, "secret") {
		t.Errorf("list output must not leak secret material, got:\n%s", out)
	}
}

// TestWebhookRm_DeletesByID: `hatch webhook rm <id>` calls the API with the
// id and confirms removal.
func TestWebhookRm_DeletesByID(t *testing.T) {
	mock := &mockAPIClient{}
	withTestDeps(t, mock)

	out, err := captureOutput(func() error { return runRm(rmCmd, []string{"wh-1"}) })
	if err != nil {
		t.Fatalf("runRm: %v", err)
	}
	if mock.deletedID != "wh-1" {
		t.Errorf("DeleteWebhook called with %q, want wh-1", mock.deletedID)
	}
	if !strings.Contains(strings.ToLower(out), "remov") {
		t.Errorf("rm output must confirm removal, got:\n%s", out)
	}
}

// TestWebhookTest_TriggersPing: `hatch webhook test <id>` calls the ping
// route and reports the outcome.
func TestWebhookTest_TriggersPing(t *testing.T) {
	mock := &mockAPIClient{}
	withTestDeps(t, mock)

	out, err := captureOutput(func() error { return runTest(testCmd, []string{"wh-1"}) })
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if mock.testedID != "wh-1" {
		t.Errorf("TestWebhook called with %q, want wh-1", mock.testedID)
	}
	if !strings.Contains(strings.ToLower(out), "ping") {
		t.Errorf("test output must mention the ping delivery, got:\n%s", out)
	}
}
