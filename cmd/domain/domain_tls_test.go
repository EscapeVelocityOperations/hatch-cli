package domain

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
)

// h-zqbr: the TLS column says whether HTTPS works; STATUS only says ownership
// is proven. Colors are off under go test (stdout is not a terminal).
func TestTLSCell(t *testing.T) {
	exp := time.Date(2026, 12, 28, 10, 0, 0, 0, time.UTC)
	retry := time.Date(2026, 9, 29, 16, 40, 0, 0, time.UTC)

	tests := []struct {
		name string
		d    api.Domain
		want string
	}{
		{"old API without tls fields", api.Domain{Domain: "a.com", Status: "active", Verified: true}, "unknown"},
		{"issued with expiry", api.Domain{TLSStatus: "issued", TLS: &api.DomainTLS{Status: "issued", ExpiresAt: &exp}}, "issued (until 2026-12-28)"},
		{"issued without expiry", api.Domain{TLSStatus: "issued"}, "issued"},
		{"rate limited with retry", api.Domain{TLSStatus: "rate_limited", TLS: &api.DomainTLS{Status: "rate_limited", RetryAfter: &retry}}, "rate_limited (retry 16:40Z)"},
		{"awaiting traffic", api.Domain{TLSStatus: "awaiting_traffic"}, "awaiting_traffic"},
		{"awaiting verification", api.Domain{TLSStatus: "awaiting_verification"}, "awaiting_verification"},
		{"pending", api.Domain{TLSStatus: "pending"}, "pending"},
		{"error", api.Domain{TLSStatus: "error"}, "error"},
		{"expired", api.Domain{TLSStatus: "expired"}, "expired"},
		{"api unknown", api.Domain{TLSStatus: "unknown"}, "unknown"},
		{"tls object only", api.Domain{TLS: &api.DomainTLS{Status: "pending"}}, "pending"},
		{"future state passes through", api.Domain{TLSStatus: "revoked"}, "revoked"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tlsCell(tt.d); got != tt.want {
				t.Errorf("tlsCell() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDomainDecodesTLSFields(t *testing.T) {
	body := `{"domain":"shop.example.com","status":"active","verified":true,
		"tls_status":"rate_limited",
		"tls":{"status":"rate_limited","retry_after":"2026-09-29T16:40:00Z","detail":"issuance budget exhausted"}}`
	var d api.Domain
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		t.Fatal(err)
	}
	if d.Status != "active" || !d.Verified {
		t.Errorf("ownership fields changed: status=%q verified=%v", d.Status, d.Verified)
	}
	if d.TLSStatus != "rate_limited" || d.TLS == nil || d.TLS.RetryAfter == nil || d.TLS.Detail == "" {
		t.Errorf("tls fields not decoded: %+v %+v", d.TLSStatus, d.TLS)
	}

	var old api.Domain
	if err := json.Unmarshal([]byte(`{"domain":"a.com","status":"active","verified":true}`), &old); err != nil {
		t.Fatal(err)
	}
	if old.TLSStatus != "" || old.TLS != nil {
		t.Errorf("old API response must leave tls fields empty: %+v", old)
	}
}

func TestListDomains_TLSColumn(t *testing.T) {
	var gotSlug string
	deps = &Deps{
		GetToken: func() (string, error) { return "tok", nil },
		ListApps: func(token string) ([]api.App, error) {
			return []api.App{{Name: "test-app", Slug: "test-app-x1y2"}}, nil
		},
		ListDomains: func(token, slug string) ([]api.Domain, error) {
			gotSlug = slug
			return []api.Domain{
				{Domain: "new.example.com", Status: "active", Verified: true, TLSStatus: "awaiting_traffic",
					TLS: &api.DomainTLS{Status: "awaiting_traffic", Detail: "certificate is issued on the first HTTPS request after DNS points to hatch"}},
				{Domain: "legacy.example.com", Status: "active", Verified: true},
			}, nil
		},
	}
	defer func() { deps = defaultDeps() }()

	out := captureStdout(t, func() {
		if err := runList("test-app"); err != nil {
			t.Fatalf("runList: %v", err)
		}
	})

	if gotSlug != "test-app-x1y2" {
		t.Errorf("ListDomains slug = %q, want resolved slug %q", gotSlug, "test-app-x1y2")
	}

	lines := strings.Split(out, "\n")
	if len(lines) == 0 || !hasField(lines[0], "TLS") {
		t.Fatalf("table header has no TLS column:\n%s", out)
	}
	tlsIdx := fieldIndex(lines[0], "TLS")
	wantTLS := map[string]string{
		"new.example.com":    "awaiting_traffic",
		"legacy.example.com": "unknown",
	}
	for domain, want := range wantTLS {
		row := lineStartingWith(lines, domain)
		if row == "" {
			t.Errorf("no table row for %s:\n%s", domain, out)
			continue
		}
		f := strings.Fields(row)
		if tlsIdx >= len(f) || f[tlsIdx] != want {
			t.Errorf("TLS cell for %s = %v, want %q (row %q)", domain, f, want, row)
		}
	}
	if !strings.Contains(out, "new.example.com: certificate is issued on the first HTTPS request after DNS points to hatch") {
		t.Errorf("missing TLS detail line for new.example.com:\n%s", out)
	}
	if strings.Contains(out, "legacy.example.com:") {
		t.Errorf("unexpected detail line for domain without TLS detail:\n%s", out)
	}
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what
// fn wrote there.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() {
		os.Stdout = old
	}()
	fn()
	os.Stdout = old
	w.Close()
	return <-done
}

func hasField(line, name string) bool { return fieldIndex(line, name) >= 0 }

func fieldIndex(line, name string) int {
	for i, f := range strings.Fields(line) {
		if f == name {
			return i
		}
	}
	return -1
}

func lineStartingWith(lines []string, prefix string) string {
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), prefix+" ") {
			return l
		}
	}
	return ""
}
