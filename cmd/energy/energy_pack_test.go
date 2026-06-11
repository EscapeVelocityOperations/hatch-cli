package energy

// TDD red suite for energy packs (h-gbo48, feature h-vlmt8, spec h-avt0u).
// Spec AC#4: `hatch energy` shows all four buckets; `hatch energy buy`
// returns a checkout URL. Red until impl-cli (h-sjoev). House deps-swap
// style (cmd/deploy/deploy_test.go) instead of golden files — flagged on
// the plan for review.

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
)

func captureStderr(fn func()) string {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	fn()

	w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// T-007 — `hatch energy` renders the pack bucket alongside daily/weekly.
func TestWriteAccountEnergy_ShowsPackBucket(t *testing.T) {
	energy := &api.EnergyStatus{
		Tier:            "free",
		DailyRemaining:  78,
		DailyLimit:      120,
		WeeklyRemaining: 300,
		WeeklyLimit:     960,
		PackRemaining:   737, // sentinel — collides with nothing else
		ResetsAt:        "2025-01-02T00:00:00Z",
	}

	var buf bytes.Buffer
	writeAccountEnergy(&buf, energy)
	out := buf.String()

	if !strings.Contains(out, "Daily") {
		t.Errorf("output lost the Daily bucket line:\n%s", out)
	}
	if !strings.Contains(out, "Weekly") {
		t.Errorf("output lost the Weekly bucket line:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "pack") {
		t.Errorf("output does not label the pack bucket:\n%s", out)
	}
	if !strings.Contains(out, "737") {
		t.Errorf("output does not show the pack balance (want 737 rendered):\n%s", out)
	}
}

// T-008 — `hatch energy buy` creates a checkout session and prints the URL.
func TestEnergyBuy_PrintsCheckoutURL(t *testing.T) {
	called := false
	deps = &Deps{
		GetToken: func() (string, error) { return "tok-123", nil },
		CreatePackCheckout: func(token string) (*api.EnergyPackCheckoutResponse, error) {
			called = true
			if token != "tok-123" {
				t.Errorf("CreatePackCheckout token = %q, want \"tok-123\"", token)
			}
			return &api.EnergyPackCheckoutResponse{
				CheckoutURL: "https://checkout.stripe.com/c/pay/cs_test_737",
				AmountEur:   "3.00",
				Minutes:     1000,
			}, nil
		},
	}
	defer func() { deps = defaultDeps() }()

	var runErr error
	out := captureStderr(func() { runErr = runBuy(nil, nil) })

	if runErr != nil {
		t.Fatalf("runBuy returned error: %v", runErr)
	}
	if !called {
		t.Fatal("runBuy never called CreatePackCheckout (stub not implemented)")
	}
	if !strings.Contains(out, "https://checkout.stripe.com/c/pay/cs_test_737") {
		t.Errorf("output does not contain the checkout URL:\n%s", out)
	}
}

// T-008 — not logged in: actionable error, no checkout attempt.
func TestEnergyBuy_NotLoggedIn(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "", errors.New("no token file") },
		CreatePackCheckout: func(token string) (*api.EnergyPackCheckoutResponse, error) {
			t.Fatal("CreatePackCheckout must not be called when not logged in")
			return nil, nil
		},
	}
	defer func() { deps = defaultDeps() }()

	err := runBuy(nil, nil)

	if err == nil {
		t.Fatal("expected not-logged-in error, got nil")
	}
	if !strings.Contains(err.Error(), "hatch login") {
		t.Errorf("error %q does not point the user at 'hatch login'", err.Error())
	}
}
