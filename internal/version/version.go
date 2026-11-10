// Package version holds the CLI version string, set via ldflags at build
// time. It exists separately from cmd/root so mcpserver can read it without
// creating an import cycle (cmd/root -> cmd/mcp -> internal/mcpserver).
package version

var version = "dev"

// Version returns the current CLI version string.
func Version() string {
	return version
}
