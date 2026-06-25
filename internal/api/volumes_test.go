package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEnableVolume(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotAuth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"size_mb":2048,"used_mb":0,"status":"active","mount":"/data"}`))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	if err := c.EnableVolume("my-app", 2048); err != nil {
		t.Fatalf("EnableVolume: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/v1/apps/my-app/volume" {
		t.Errorf("request = %s %s, want POST /v1/apps/my-app/volume", gotMethod, gotPath)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth = %q", gotAuth)
	}
	if body["size_mb"] != float64(2048) {
		t.Errorf("body size_mb = %v, want 2048", body["size_mb"])
	}
}

func TestEnableVolume_OverCapError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "size_mb must be between 1 and the tier cap", http.StatusUnprocessableEntity)
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	err := c.EnableVolume("my-app", 999999)
	if err == nil || !strings.Contains(err.Error(), "422") {
		t.Errorf("EnableVolume error = %v, want a 422 API error", err)
	}
}

func TestGetVolume(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"size_mb":1024,"used_mb":37,"status":"active","mount":"/data","over_quota":false}`))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	v, err := c.GetVolume("my-app")
	if err != nil {
		t.Fatalf("GetVolume: %v", err)
	}
	if gotMethod != "GET" || gotPath != "/v1/apps/my-app/volume" {
		t.Errorf("request = %s %s, want GET /v1/apps/my-app/volume", gotMethod, gotPath)
	}
	if v.SizeMB != 1024 || v.UsedMB != 37 || v.Status != "active" || v.Mount != "/data" {
		t.Errorf("volume = %+v, want {1024 37 active /data ...}", v)
	}
}

func TestDisableVolume(t *testing.T) {
	tests := []struct {
		name    string
		now     bool
		wantURL string
	}{
		{"grace", false, "/v1/apps/my-app/volume"},
		{"now", true, "/v1/apps/my-app/volume?now=true"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotMethod, gotURL string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotURL = r.Method, r.URL.RequestURI()
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte(`{"size_mb":1024,"used_mb":0,"status":"grace_deleting","mount":"/data"}`))
			}))
			defer server.Close()

			c := NewClient("tok123")
			c.host = server.URL

			if err := c.DisableVolume("my-app", tc.now); err != nil {
				t.Fatalf("DisableVolume: %v", err)
			}
			if gotMethod != "DELETE" || gotURL != tc.wantURL {
				t.Errorf("request = %s %s, want DELETE %s", gotMethod, gotURL, tc.wantURL)
			}
		})
	}
}
