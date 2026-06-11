// Package webhook implements `hatch webhook add|list|rm|test` — managing
// outbound deploy webhooks (spec h-xv5s7). Stub for h-7wgqx (tests-first);
// the impl-cli step replaces the run bodies and wires the API client.
package webhook

import (
	"errors"

	"github.com/spf13/cobra"
)

// Webhook mirrors the API's webhook resource as the CLI consumes it.
type Webhook struct {
	ID         string
	URL        string
	Events     []string
	Active     bool
	LastStatus string
}

// APIClient is the webhook surface of the Hatch API.
type APIClient interface {
	// CreateWebhook returns the created webhook and the plaintext signing
	// secret — the only time the secret is ever available.
	CreateWebhook(slug, url string, events []string) (*Webhook, string, error)
	ListWebhooks(slug string) ([]Webhook, error)
	DeleteWebhook(slug, id string) error
	// TestWebhook triggers the signed ping delivery server-side.
	TestWebhook(slug, id string) error
}

// Deps holds injectable dependencies for testing (cmd/deploy pattern).
type Deps struct {
	GetToken     func() (string, error)
	GetCwd       func() (string, error)
	NewAPIClient func(token string) APIClient
}

var deps = defaultDeps()

func defaultDeps() *Deps {
	return &Deps{}
}

// Cmd is the `hatch webhook` command group.
var Cmd = &cobra.Command{
	Use:   "webhook",
	Short: "Manage deploy webhooks for this app",
}

var addCmd = &cobra.Command{
	Use:   "add <url>",
	Short: "Register a webhook (the signing secret is shown once)",
	Args:  cobra.ExactArgs(1),
	RunE:  runAdd,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List this app's webhooks",
	RunE:  runList,
}

var rmCmd = &cobra.Command{
	Use:   "rm <id>",
	Short: "Remove a webhook",
	Args:  cobra.ExactArgs(1),
	RunE:  runRm,
}

var testCmd = &cobra.Command{
	Use:   "test <id>",
	Short: "Send a signed ping event to a webhook",
	Args:  cobra.ExactArgs(1),
	RunE:  runTest,
}

func init() {
	Cmd.AddCommand(addCmd, listCmd, rmCmd, testCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	return errors.New("not implemented")
}

func runList(cmd *cobra.Command, args []string) error {
	return errors.New("not implemented")
}

func runRm(cmd *cobra.Command, args []string) error {
	return errors.New("not implemented")
}

func runTest(cmd *cobra.Command, args []string) error {
	return errors.New("not implemented")
}
