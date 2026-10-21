package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/sessionttl"
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
		mcp.WithString("session_ttl",
			mcp.Description("How long a magic-link sign-in lasts: 1h to 30d (e.g. \"12h\", \"30d\"), or \"default\" (12h). Omit to keep the current setting."),
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
	var ttl *int
	if raw := req.GetString("session_ttl", ""); raw != "" {
		secs, err := sessionttl.Parse(raw)
		if err != nil {
			return toolError("failed to set email protection: %v", err)
		}
		ttl = &secs
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to set email protection: %v", err)
	}

	ep, err := client.SetEmailProtection(slug, emails, domains, ttl)
	if err != nil {
		return toolError("failed to set email protection: %v", err)
	}

	return mcp.NewToolResultText(fmt.Sprintf(
		"Email protection enabled for %s.\nEmails: %s\nDomains: %s\nSession: %s",
		slug, strings.Join(ep.Emails, ", "), strings.Join(ep.Domains, ", "), sessionttl.Format(ep.SessionTTLSeconds),
	)), nil
}

// --- get_email_protection ---

func getEmailProtectionTool() mcp.Tool {
	return mcp.NewTool("get_email_protection",
		mcp.WithDescription("Get the current email-allowlist protection state for an egg, including session_ttl_seconds (how long a magic-link sign-in lasts)."),
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

// --- revoke_egg_sessions ---

func revokeEggSessionsTool() mcp.Tool {
	return mcp.NewTool("revoke_egg_sessions",
		mcp.WithDescription("Sign every visitor out of a protected egg (password and magic-link sessions) by rotating its session secret. Visitors must sign in again."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) whose sessions to revoke"),
		),
	)
}

func revokeEggSessionsHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return toolError("failed to revoke sessions: missing required parameter 'app'")
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to revoke sessions: %v", err)
	}

	if err := client.RevokeSessions(slug); err != nil {
		return toolError("failed to revoke sessions: %v", err)
	}

	return mcp.NewToolResultText(fmt.Sprintf("All visitor sessions of %s revoked.", slug)), nil
}
