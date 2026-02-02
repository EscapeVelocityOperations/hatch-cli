package mcp

import (
	"fmt"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/mcpserver"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

func NewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP tool server for AI assistants",
		Long:  "Start a Model Context Protocol (MCP) server over stdio. This exposes Hatch CLI tools for use by AI assistants like Claude.",
		RunE:  runMCP,
	}
}

func runMCP(cmd *cobra.Command, args []string) error {
	s := mcpserver.NewServer()
	if err := server.ServeStdio(s); err != nil {
		return fmt.Errorf("MCP server error: %w", err)
	}
	return nil
}
