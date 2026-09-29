package domain

import (
	"encoding/json"
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
	deps = &Deps{
		GetToken: func() (string, error) { return "tok", nil },
		ListDomains: func(token, slug string) ([]api.Domain, error) {
			return []api.Domain{
				{Domain: "new.example.com", Status: "active", Verified: true, TLSStatus: "awaiting_traffic",
					TLS: &api.DomainTLS{Status: "awaiting_traffic", Detail: "certificate is issued on the first HTTPS request after DNS points to hatch"}},
				{Domain: "legacy.example.com", Status: "active", Verified: true},
			}, nil
		},
	}
	defer func() { deps = defaultDeps() }()

	if err := runList("test-app"); err != nil {
		t.Fatalf("runList: %v", err)
	}
}
