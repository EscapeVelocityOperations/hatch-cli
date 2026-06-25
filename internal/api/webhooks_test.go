package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateWebhook(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotAuth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"wh_1","url":"https://x.test/hook","events":["deploy"],"secret":"whsec_abc"}`))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	wh, err := c.CreateWebhook("my-app", "https://x.test/hook", []string{"deploy"})
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/v1/apps/my-app/webhooks" {
		t.Errorf("request = %s %s, want POST /v1/apps/my-app/webhooks", gotMethod, gotPath)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth = %q, want Bearer tok123", gotAuth)
	}
	if body["url"] != "https://x.test/hook" {
		t.Errorf("body url = %v", body["url"])
	}
	if wh.ID != "wh_1" || wh.Secret != "whsec_abc" {
		t.Errorf("webhook = %+v, want id wh_1 + secret whsec_abc", wh)
	}
}

func TestListWebhooks(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"wh_1","url":"https://a.test","events":["deploy"]},{"id":"wh_2","url":"https://b.test","events":["deploy.failed"]}]`))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	whs, err := c.ListWebhooks("my-app")
	if err != nil {
		t.Fatalf("ListWebhooks: %v", err)
	}
	if gotMethod != "GET" || gotPath != "/v1/apps/my-app/webhooks" {
		t.Errorf("request = %s %s, want GET /v1/apps/my-app/webhooks", gotMethod, gotPath)
	}
	if len(whs) != 2 || whs[0].ID != "wh_1" || whs[1].ID != "wh_2" {
		t.Errorf("webhooks = %+v, want two rows wh_1/wh_2", whs)
	}
}

func TestDeleteWebhook(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	if err := c.DeleteWebhook("my-app", "wh_1"); err != nil {
		t.Fatalf("DeleteWebhook: %v", err)
	}
	if gotMethod != "DELETE" || gotPath != "/v1/apps/my-app/webhooks/wh_1" {
		t.Errorf("request = %s %s, want DELETE /v1/apps/my-app/webhooks/wh_1", gotMethod, gotPath)
	}
}

func TestTestWebhook(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	if err := c.TestWebhook("my-app", "wh_1"); err != nil {
		t.Fatalf("TestWebhook: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/v1/apps/my-app/webhooks/wh_1/test" {
		t.Errorf("request = %s %s, want POST /v1/apps/my-app/webhooks/wh_1/test", gotMethod, gotPath)
	}
}

// TestDeleteWebhook_APIError: a 4xx is surfaced as an error, not swallowed.
func TestDeleteWebhook_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	err := c.DeleteWebhook("my-app", "nope")
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("DeleteWebhook error = %v, want a 404 API error", err)
	}
}
