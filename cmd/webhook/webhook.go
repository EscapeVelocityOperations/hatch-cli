// Package webhook implements `hatch webhook add|list|rm|test` — managing
// outbound deploy webhooks (spec h-xv5s7). Stub for h-7wgqx (tests-first);
// the impl-cli step replaces the run bodies and wires the API client.
package webhook

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/resolve"
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
	return &Deps{
		GetToken: auth.GetToken,
		GetCwd:   os.Getwd,
		// NewAPIClient stays nil until the hatch-api webhook endpoints
		// (feature h-2o06e) land and an *api.Client adapter is wired —
		// tracked as a follow-up. resolveApp() guards on nil so an un-wired
		// invocation fails loudly with a clear message instead of panicking.
		NewAPIClient: nil,
	}
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
	addCmd.Flags().StringSliceP("event", "e", []string{"deploy"},
		"Event(s) that trigger this webhook")
	Cmd.AddCommand(addCmd, listCmd, rmCmd, testCmd)
}

// resolveApp returns the webhook API client and the current app slug, or a
// friendly error (not logged in / not yet wired / no app in this directory).
func resolveApp() (APIClient, string, error) {
	token, err := deps.GetToken()
	if err != nil || token == "" {
		return nil, "", errors.New("not logged in (run 'hatch login' first)")
	}
	if deps.NewAPIClient == nil {
		return nil, "", errors.New("webhook commands are not yet wired to the API " +
			"(pending hatch-api webhook endpoints, feature h-2o06e)")
	}

	dir := "."
	if deps.GetCwd != nil {
		d, err := deps.GetCwd()
		if err != nil {
			return nil, "", fmt.Errorf("resolving working directory: %w", err)
		}
		dir = d
	}
	slug := resolve.SlugFromDir(dir)
	if slug == "" {
		return nil, "", errors.New("no app found here — run from an app directory " +
			"with a .hatch.toml (or 'hatch init' first)")
	}

	return deps.NewAPIClient(token), slug, nil
}

func runAdd(cmd *cobra.Command, args []string) error {
	client, slug, err := resolveApp()
	if err != nil {
		return err
	}

	events, _ := cmd.Flags().GetStringSlice("event")
	if len(events) == 0 {
		events = []string{"deploy"}
	}

	wh, secret, err := client.CreateWebhook(slug, args[0], events)
	if err != nil {
		return fmt.Errorf("creating webhook: %w", err)
	}

	fmt.Printf("Webhook %s registered for %s\n", wh.ID, wh.URL)
	fmt.Printf("Events: %s\n", strings.Join(wh.Events, ", "))
	fmt.Println()
	fmt.Printf("  Signing secret: %s\n", secret)
	fmt.Println("  Save this now — it is shown only once and cannot be retrieved later.")
	return nil
}

func runList(cmd *cobra.Command, args []string) error {
	client, slug, err := resolveApp()
	if err != nil {
		return err
	}

	hooks, err := client.ListWebhooks(slug)
	if err != nil {
		return fmt.Errorf("listing webhooks: %w", err)
	}
	if len(hooks) == 0 {
		fmt.Println("No webhooks registered for this app.")
		return nil
	}

	fmt.Printf("%-12s  %-40s  %-16s  %s\n", "ID", "URL", "EVENTS", "STATUS")
	for _, h := range hooks {
		status := h.LastStatus
		if status == "" {
			status = "—"
		}
		if !h.Active {
			status = "disabled"
		}
		fmt.Printf("%-12s  %-40s  %-16s  %s\n",
			h.ID, h.URL, strings.Join(h.Events, ","), status)
	}
	return nil
}

func runRm(cmd *cobra.Command, args []string) error {
	client, slug, err := resolveApp()
	if err != nil {
		return err
	}

	id := args[0]
	if err := client.DeleteWebhook(slug, id); err != nil {
		return fmt.Errorf("removing webhook %s: %w", id, err)
	}

	fmt.Printf("Removed webhook %s.\n", id)
	return nil
}

func runTest(cmd *cobra.Command, args []string) error {
	client, slug, err := resolveApp()
	if err != nil {
		return err
	}

	id := args[0]
	if err := client.TestWebhook(slug, id); err != nil {
		return fmt.Errorf("testing webhook %s: %w", id, err)
	}

	fmt.Printf("Sent a signed ping event to webhook %s.\n", id)
	return nil
}
