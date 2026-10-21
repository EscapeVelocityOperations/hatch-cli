package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSetEmailProtection_SessionTTL (h-pbhn): nil omits session_ttl_seconds
// (server keeps the stored value), 0 and values are sent explicitly; the
// effective TTL is decoded from the response.
func TestSetEmailProtection_SessionTTL(t *testing.T) {
	zero, month := 0, 2592000
	for _, tc := range []struct {
		name    string
		in      *int
		present bool
		want    float64
	}{
		{"nil omitted", nil, false, 0},
		{"zero sent", &zero, true, 0},
		{"value sent", &month, true, 2592000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&body)
				_, _ = w.Write([]byte(`{"email_protected":true,"emails":["a@b.com"],"domains":[],"session_ttl_seconds":2592000}`))
			}))
			defer server.Close()
			c := NewClient("tok")
			c.host = server.URL

			ep, err := c.SetEmailProtection("my-app", []string{"a@b.com"}, nil, tc.in)
			if err != nil {
				t.Fatalf("SetEmailProtection: %v", err)
			}
			got, present := body["session_ttl_seconds"]
			if present != tc.present || (present && got != tc.want) {
				t.Errorf("session_ttl_seconds = %v (present %v), want %v (present %v)", got, present, tc.want, tc.present)
			}
			if ep.SessionTTLSeconds != 2592000 {
				t.Errorf("decoded SessionTTLSeconds = %d", ep.SessionTTLSeconds)
			}
		})
	}
}

func TestRevokeSessions(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_, _ = w.Write([]byte(`{"revoked":true}`))
	}))
	defer server.Close()
	c := NewClient("tok")
	c.host = server.URL

	if err := c.RevokeSessions("my-app"); err != nil {
		t.Fatalf("RevokeSessions: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/v1/apps/my-app/sessions/revoke" {
		t.Errorf("request = %s %s, want POST /v1/apps/my-app/sessions/revoke", gotMethod, gotPath)
	}
}

func TestRevokeSessions_ConflictIsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"egg is not protected"}`, http.StatusConflict)
	}))
	defer server.Close()
	c := NewClient("tok")
	c.host = server.URL

	if err := c.RevokeSessions("my-app"); err == nil {
		t.Fatal("want an error on 409")
	}
}
