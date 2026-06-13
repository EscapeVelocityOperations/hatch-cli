package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PacksCheckoutResponse is returned by POST /billing/packs/checkout.
type PacksCheckoutResponse struct {
	CheckoutURL string `json:"checkout_url"`
	Size        string `json:"size"`
	Minutes     int    `json:"minutes"`
	AmountEur   string `json:"amount_eur"`
}

// PackPurchase is one entry of the user's pack purchase history.
type PackPurchase struct {
	ID          string `json:"id"`
	Minutes     int    `json:"minutes"`
	AmountCents int    `json:"amount_cents"`
	Currency    string `json:"currency"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
}

// ListPacksResponse is returned by GET /billing/packs.
type ListPacksResponse struct {
	PackMinutes int            `json:"pack_minutes"`
	Purchases   []PackPurchase `json:"purchases"`
}

// PacksCheckout creates a Stripe checkout session for an energy pack purchase.
// Returns a checkout URL to open in the browser.
func (c *Client) PacksCheckout(size string) (*PacksCheckoutResponse, error) {
	body := fmt.Sprintf(`{"size":%q}`, size)
	resp, err := c.do("POST", "/billing/packs/checkout", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result PacksCheckoutResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}

// ListPacks returns the user's non-expiring pack balance and purchase history.
func (c *Client) ListPacks() (*ListPacksResponse, error) {
	resp, err := c.do("GET", "/billing/packs", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result ListPacksResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}
