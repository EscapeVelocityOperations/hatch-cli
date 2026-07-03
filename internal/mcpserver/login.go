package mcpserver

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// --- login ---

func loginTool() mcp.Tool {
	return mcp.NewTool("login",
		mcp.WithDescription("Authenticate with Hatch via browser sign-in or account creation (signup==login). Opens a browser to gethatch.eu; on success returns your plan summary. Call this whenever another tool reports 'not authenticated'."),
	)
}

func loginHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return toolError("failed to login: not yet implemented")
}
