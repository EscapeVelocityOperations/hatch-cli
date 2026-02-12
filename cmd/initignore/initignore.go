package initignore

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
	"github.com/spf13/cobra"
)

var runtimeFlag string

// templates maps runtimes to starter .hatchignore content.
var templates = map[string]string{
	"node": `# .hatchignore — files to exclude from deploy artifact
node_modules/
src/
.next/
*.md
tests/
test/
__tests__/
*.test.js
*.test.ts
*.spec.js
*.spec.ts
tsconfig.json
`,
	"bun": `# .hatchignore — files to exclude from deploy artifact
node_modules/
src/
*.md
tests/
test/
__tests__/
*.test.ts
*.spec.ts
tsconfig.json
`,
	"python": `# .hatchignore — files to exclude from deploy artifact
__pycache__/
*.pyc
.venv/
venv/
tests/
test/
*.md
setup.py
setup.cfg
pyproject.toml
`,
	"go": `# .hatchignore — files to exclude from deploy artifact
*.go
*_test.go
go.mod
go.sum
vendor/
*.md
Makefile
`,
	"rust": `# .hatchignore — files to exclude from deploy artifact
src/
target/
Cargo.toml
Cargo.lock
*.md
tests/
`,
	"static": `# .hatchignore — files to exclude from deploy artifact
node_modules/
src/
*.ts
*.tsx
*.jsx
*.md
tests/
test/
tsconfig.json
package.json
package-lock.json
`,
	"php": `# .hatchignore — files to exclude from deploy artifact
node_modules/
vendor/
tests/
*.md
composer.lock
phpunit.xml
`,
}

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init-ignore",
		Short: "Generate a starter .hatchignore file",
		Long: `Generate a starter .hatchignore file based on your project's runtime.

The .hatchignore file controls which files are excluded from deploy artifacts,
similar to .gitignore. Safety defaults (.git, .env, .DS_Store) are always
applied regardless of the file contents.

Examples:
  hatch init-ignore                  # auto-detect runtime
  hatch init-ignore --runtime node   # generate for Node.js`,
		RunE: runInitIgnore,
	}
	cmd.Flags().StringVar(&runtimeFlag, "runtime", "", "runtime to generate template for (node, python, go, rust, php, bun, static)")
	return cmd
}

func runInitIgnore(cmd *cobra.Command, args []string) error {
	rt := runtimeFlag
	if rt == "" {
		rt = detectRuntime(".")
	}
	if rt == "" {
		return fmt.Errorf("could not detect runtime. Use --runtime to specify one (node, python, go, rust, php, bun, static)")
	}

	tmpl, ok := templates[rt]
	if !ok {
		return fmt.Errorf("no template for runtime %q (valid: node, python, go, rust, php, bun, static)", rt)
	}

	path := filepath.Join(".", ".hatchignore")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf(".hatchignore already exists. Remove it first or edit it manually")
	}

	if err := os.WriteFile(path, []byte(tmpl), 0644); err != nil {
		return fmt.Errorf("writing .hatchignore: %w", err)
	}

	ui.Success(fmt.Sprintf("Created .hatchignore for %s runtime", rt))
	ui.Info("Review and customize it, then run 'hatch deploy'")
	return nil
}

// detectRuntime guesses the runtime from files in the directory.
func detectRuntime(dir string) string {
	has := func(name string) bool {
		_, err := os.Stat(filepath.Join(dir, name))
		return err == nil
	}

	switch {
	case has("go.mod"):
		return "go"
	case has("Cargo.toml"):
		return "rust"
	case has("requirements.txt") || has("pyproject.toml") || has("Pipfile"):
		return "python"
	case has("composer.json"):
		return "php"
	case has("bun.lockb") || has("bunfig.toml"):
		return "bun"
	case has("package.json"):
		return "node"
	case has("index.html"):
		return "static"
	default:
		return ""
	}
}
