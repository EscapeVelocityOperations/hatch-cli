package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPacksCheckout(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"checkout_url":"https://checkout.stripe.com/x","size":"standard","minutes":1000,"amount_eur":"3.00"}`))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	res, err := c.PacksCheckout("standard")
	if err != nil {
		t.Fatalf("PacksCheckout: %v", err)
	}
	if gotPath != "/v1/billing/packs/checkout" {
		t.Errorf("POST path = %q, want /v1/billing/packs/checkout", gotPath)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("Authorization = %q, want Bearer tok123", gotAuth)
	}
	if !strings.Contains(gotBody, `"size":"standard"`) {
		t.Errorf("body = %q, want it to contain size", gotBody)
	}
	if res.CheckoutURL != "https://checkout.stripe.com/x" {
		t.Errorf("CheckoutURL = %q", res.CheckoutURL)
	}
	if res.Minutes != 1000 {
		t.Errorf("Minutes = %d, want 1000", res.Minutes)
	}
}

func TestListPacks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/billing/packs" {
			t.Errorf("GET path = %q, want /v1/billing/packs", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pack_minutes":1000,"purchases":[{"id":"p1","minutes":1000,"amount_cents":300,"currency":"eur","status":"paid","created_at":"2026-06-13T00:00:00Z"}]}`))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	res, err := c.ListPacks()
	if err != nil {
		t.Fatalf("ListPacks: %v", err)
	}
	if res.PackMinutes != 1000 {
		t.Errorf("PackMinutes = %d, want 1000", res.PackMinutes)
	}
	if len(res.Purchases) != 1 {
		t.Fatalf("purchases len = %d, want 1", len(res.Purchases))
	}
	if res.Purchases[0].Status != "paid" || res.Purchases[0].Minutes != 1000 {
		t.Errorf("purchase = %+v, want minutes=1000 status=paid", res.Purchases[0])
	}
}
