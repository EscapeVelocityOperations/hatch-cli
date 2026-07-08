package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
