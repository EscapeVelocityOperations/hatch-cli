package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/allowlist"
	"github.com/mark3labs/mcp-go/mcp"
)

// --- set_email_protection ---

func setEmailProtectionTool() mcp.Tool {
	return mcp.NewTool("set_email_protection",
		mcp.WithDescription("Enable email-allowlist protection for an egg: visitors sign in via a magic link sent to an allowed email address or domain. Replaces the full current allowlist (not a per-item add)."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to protect"),
		),
		mcp.WithArray("emails",
			mcp.Description("Exact email addresses to allow"),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithArray("domains",
			mcp.Description("Email domains to allow (with or without a leading @)"),
			mcp.Items(map[string]any{"type": "string"}),
		),
	)
}

func setEmailProtectionHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return toolError("failed to set email protection: missing required parameter 'app'")
	}
	emails := req.GetStringSlice("emails", nil)
	domains := req.GetStringSlice("domains", nil)
	if len(emails) == 0 && len(domains) == 0 {
		return toolError("failed to set email protection: specify at least one of 'emails' or 'domains'")
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to set email protection: %v", err)
	}

	ep, err := client.SetEmailProtection(slug, emails, domains)
	if err != nil {
		return toolError("failed to set email protection: %v", err)
	}

	return mcp.NewToolResultText(fmt.Sprintf(
		"Email protection enabled for %s.\nEmails: %s\nDomains: %s",
		slug, strings.Join(ep.Emails, ", "), strings.Join(ep.Domains, ", "),
	)), nil
}

// --- get_email_protection ---

func getEmailProtectionTool() mcp.Tool {
	return mcp.NewTool("get_email_protection",
		mcp.WithDescription("Get the current email-allowlist protection state for an egg."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to check"),
		),
	)
}

func getEmailProtectionHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return toolError("failed to get email protection: missing required parameter 'app'")
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to get email protection: %v", err)
	}

	ep, err := client.GetEmailProtection(slug)
	if err != nil {
		return toolError("failed to get email protection: %v", err)
	}

	data, err := json.Marshal(ep)
	if err != nil {
		return toolError("failed to get email protection: %v", err)
	}
	return mcp.NewToolResultText(string(data)), nil
}

// --- disable_email_protection ---

func disableEmailProtectionTool() mcp.Tool {
	return mcp.NewTool("disable_email_protection",
		mcp.WithDescription("Disable email-allowlist protection for an egg."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to unprotect"),
		),
	)
}

func disableEmailProtectionHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return toolError("failed to disable email protection: missing required parameter 'app'")
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to disable email protection: %v", err)
	}

	if err := client.DeleteEmailProtection(slug); err != nil {
		return toolError("failed to disable email protection: %v", err)
	}

	return mcp.NewToolResultText(fmt.Sprintf("Email protection disabled for %s.", slug)), nil
}

// --- normalization + merge helpers (add/remove) ---

// normalizeEmail trims and lowercases a single email argument so it
// compares equal to the server-normalized form, mirroring the CLI's
// normalizeEmailArg (cmd/protect/email.go).
func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// normalizeDomain trims, lowercases, and strips a leading "@" from a
// single domain argument, mirroring the CLI's normalizeDomainArg.
func normalizeDomain(s string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "@")
}

// mergeAdd appends normalized new entries to existing, skipping anything
// already present — the read-merge-write core of add_email_protection_user.
func mergeAdd(existing, add []string, normalize func(string) string) []string {
	seen := make(map[string]struct{}, len(existing))
	out := make([]string, 0, len(existing)+len(add))
	for _, e := range existing {
		if _, ok := seen[e]; !ok {
			seen[e] = struct{}{}
			out = append(out, e)
		}
	}
	for _, a := range add {
		n := normalize(a)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; !ok {
			seen[n] = struct{}{}
			out = append(out, n)
		}
	}
	return out
}

// --- add_email_protection_user ---

func addEmailProtectionUserTool() mcp.Tool {
	return mcp.NewTool("add_email_protection_user",
		mcp.WithDescription("Add email addresses and/or domains to an egg's email-allowlist, merging with the current list (does not replace it). Enables email protection if it wasn't already on."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to update"),
		),
		mcp.WithArray("emails",
			mcp.Description("Exact email addresses to add"),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithArray("domains",
			mcp.Description("Email domains to add (with or without a leading @)"),
			mcp.Items(map[string]any{"type": "string"}),
		),
	)
}

func addEmailProtectionUserHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return toolError("failed to add to email protection: missing required parameter 'app'")
	}
	emails := req.GetStringSlice("emails", nil)
	domains := req.GetStringSlice("domains", nil)
	if len(emails) == 0 && len(domains) == 0 {
		return toolError("failed to add to email protection: specify at least one of 'emails' or 'domains'")
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to add to email protection: %v", err)
	}

	current, err := client.GetEmailProtection(slug)
	if err != nil {
		return toolError("failed to add to email protection: %v", err)
	}

	newEmails := mergeAdd(current.Emails, emails, normalizeEmail)
	newDomains := mergeAdd(current.Domains, domains, normalizeDomain)

	ep, err := client.SetEmailProtection(slug, newEmails, newDomains)
	if err != nil {
		return toolError("failed to add to email protection: %v", err)
	}

	return mcp.NewToolResultText(fmt.Sprintf(
		"Email protection updated for %s.\nEmails: %s\nDomains: %s",
		slug, strings.Join(ep.Emails, ", "), strings.Join(ep.Domains, ", "),
	)), nil
}

// --- remove_email_protection_user ---

func removeEmailProtectionUserTool() mcp.Tool {
	return mcp.NewTool("remove_email_protection_user",
		mcp.WithDescription("Remove email addresses and/or domains from an egg's email-allowlist. An entry not currently on the list is a no-op, not an error."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to update"),
		),
		mcp.WithArray("emails",
			mcp.Description("Exact email addresses to remove"),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithArray("domains",
			mcp.Description("Email domains to remove (with or without a leading @)"),
			mcp.Items(map[string]any{"type": "string"}),
		),
	)
}

func removeEmailProtectionUserHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return toolError("failed to remove from email protection: missing required parameter 'app'")
	}
	emails := req.GetStringSlice("emails", nil)
	domains := req.GetStringSlice("domains", nil)
	if len(emails) == 0 && len(domains) == 0 {
		return toolError("failed to remove from email protection: specify at least one of 'emails' or 'domains'")
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to remove from email protection: %v", err)
	}

	current, err := client.GetEmailProtection(slug)
	if err != nil {
		return toolError("failed to remove from email protection: %v", err)
	}

	normEmails := make([]string, len(emails))
	for i, e := range emails {
		normEmails[i] = normalizeEmail(e)
	}
	normDomains := make([]string, len(domains))
	for i, d := range domains {
		normDomains[i] = normalizeDomain(d)
	}

	newEmails := allowlist.RemoveAll(current.Emails, normEmails)
	newDomains := allowlist.RemoveAll(current.Domains, normDomains)

	ep, err := client.SetEmailProtection(slug, newEmails, newDomains)
	if err != nil {
		return toolError("failed to remove from email protection: %v", err)
	}

	msg := fmt.Sprintf(
		"Email protection updated for %s.\nEmails: %s\nDomains: %s",
		slug, strings.Join(ep.Emails, ", "), strings.Join(ep.Domains, ", "),
	)
	if ep.EmailProtected && len(ep.Emails) == 0 && len(ep.Domains) == 0 {
		msg += "\nWarning: protection is enabled but the allowlist is now empty — this blocks every visitor."
	}

	return mcp.NewToolResultText(msg), nil
}

// --- clear_email_protection ---
//
// Directive-spelled alias of disable_email_protection (h-7b9l/h-dmd4
// T-006) — same handler behavior, both tool names callable.

func clearEmailProtectionTool() mcp.Tool {
	return mcp.NewTool("clear_email_protection",
		mcp.WithDescription("Disable email-allowlist protection for an egg. Alias of disable_email_protection."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to unprotect"),
		),
	)
}

func clearEmailProtectionHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return disableEmailProtectionHandler(ctx, req)
}
