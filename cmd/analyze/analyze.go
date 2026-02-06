package analyze

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// knownNativeModules is a list of commonly used packages with native bindings.
var knownNativeModules = []string{
	"sharp",
	"bcrypt",
	"sqlite3",
	"better-sqlite3",
	"canvas",
	"node-sass",
	"libxmljs",
	"node-gyp",
	"grpc",
	"@grpc/grpc-js", // This one is pure JS actually, but grpc is native
	"leveldown",
	"sodium-native",
	"argon2",
	"farmhash",
	"xxhash",
	"fsevents",
	"esbuild", // Has native binaries
	"swc",
	"@swc/core",
	"lightningcss",
	"@parcel/css",
}

// Analysis represents the build analysis result.
type Analysis struct {
	HasNativeModules    bool     `json:"hasNativeModules"`
	NativeModules       []string `json:"nativeModules"`
	RecommendedStrategy string   `json:"recommendedStrategy"` // "local" or "remote"
	Framework           string   `json:"framework"`           // "nuxt", "next", "node", "static", "jekyll", "hugo", "unknown"
	IsStaticSite        bool     `json:"isStaticSite"`
	BuildCommand        string   `json:"buildCommand"`
	OutputDir           string   `json:"outputDir"`
	StartCommand        string   `json:"startCommand"`
	HasDockerfile       bool     `json:"hasDockerfile"`
	NodeVersion         string   `json:"nodeVersion,omitempty"`
}

var jsonOutput bool

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Analyze project for build strategy",
		Long:  "Analyze the current directory to detect framework, build command, output directory, and native modules.",
		RunE:  runAnalyze,
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	analysis, err := AnalyzeProject(".")
	if err != nil {
		return err
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(analysis)
	}

	// Human-readable output
	fmt.Printf("Framework:            %s\n", analysis.Framework)
	fmt.Printf("Build command:        %s\n", analysis.BuildCommand)
	fmt.Printf("Output directory:     %s\n", analysis.OutputDir)
	fmt.Printf("Start command:        %s\n", analysis.StartCommand)
	fmt.Printf("Has Dockerfile:       %v\n", analysis.HasDockerfile)
	fmt.Printf("Has native modules:   %v\n", analysis.HasNativeModules)
	if len(analysis.NativeModules) > 0 {
		fmt.Printf("Native modules:       %s\n", strings.Join(analysis.NativeModules, ", "))
	}
	fmt.Printf("Recommended strategy: %s\n", analysis.RecommendedStrategy)
	if analysis.IsStaticSite {
		fmt.Println("\n→ Static site detected. Use 'hatch deploy' to upload directly.")
	} else if analysis.RecommendedStrategy == "local" {
		fmt.Println("\n→ Use 'hatch deploy' to build locally and upload")
	} else {
		fmt.Println("\n→ Has native modules — build may need matching architecture")
	}

	return nil
}

// AnalyzeProject analyzes a project directory and returns build recommendations.
func AnalyzeProject(dir string) (*Analysis, error) {
	analysis := &Analysis{
		RecommendedStrategy: "local", // Default to local
		Framework:           "unknown",
	}

	// Check for Dockerfile
	if _, err := os.Stat(filepath.Join(dir, "Dockerfile")); err == nil {
		analysis.HasDockerfile = true
	}

	// Detection priority order:
	// 1. Node.js frameworks (nuxt, next, express, node)
	// 2. Go
	// 3. Python
	// 4. Rust
	// 5. Static sites (jekyll, hugo, static)

	// Check for package.json (Node.js project)
	pkgPath := filepath.Join(dir, "package.json")
	if _, err := os.Stat(pkgPath); err == nil {
		if err := analyzeNodeProject(dir, analysis); err != nil {
			return nil, err
		}
	} else if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		// Go project
		analyzeGoProject(dir, analysis)
	} else if hasPythonProject(dir) {
		// Python project
		if err := analyzePythonProject(dir, analysis); err != nil {
			return nil, err
		}
	} else if _, err := os.Stat(filepath.Join(dir, "Cargo.toml")); err == nil {
		// Rust project
		if err := analyzeRustProject(dir, analysis); err != nil {
			return nil, err
		}
	} else {
		// No known project files — check for static site
		analyzeStaticSite(dir, analysis)
	}

	// Determine recommended strategy
	if analysis.HasNativeModules {
		analysis.RecommendedStrategy = "remote"
	}

	return analysis, nil
}

func analyzeNodeProject(dir string, analysis *Analysis) error {
	pkgPath := filepath.Join(dir, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return err
	}

	var pkg struct {
		Name         string            `json:"name"`
		Scripts      map[string]string `json:"scripts"`
		Dependencies map[string]string `json:"dependencies"`
		DevDeps      map[string]string `json:"devDependencies"`
		Engines      struct {
			Node string `json:"node"`
		} `json:"engines"`
	}

	if err := json.Unmarshal(data, &pkg); err != nil {
		return fmt.Errorf("parsing package.json: %w", err)
	}

	// Detect framework
	if _, ok := pkg.Dependencies["nuxt"]; ok {
		analysis.Framework = "nuxt"
		analysis.BuildCommand = "pnpm build"
		analysis.OutputDir = ".output"
		analysis.StartCommand = "node .output/server/index.mjs"
	} else if _, ok := pkg.Dependencies["next"]; ok {
		analysis.Framework = "next"
		analysis.BuildCommand = "pnpm build"
		analysis.OutputDir = ".next"
		analysis.StartCommand = "pnpm start"
	} else if _, ok := pkg.Dependencies["express"]; ok {
		analysis.Framework = "express"
		analysis.BuildCommand = "pnpm install --prod"
		analysis.OutputDir = "."
		analysis.StartCommand = inferStartCommand(pkg.Scripts)
	} else {
		analysis.Framework = "node"
		analysis.BuildCommand = "pnpm install --prod"
		analysis.OutputDir = "."
		analysis.StartCommand = inferStartCommand(pkg.Scripts)
	}

	// Check for node version
	if pkg.Engines.Node != "" {
		analysis.NodeVersion = pkg.Engines.Node
	}

	// Detect native modules from dependencies
	allDeps := make(map[string]string)
	for k, v := range pkg.Dependencies {
		allDeps[k] = v
	}
	for k, v := range pkg.DevDeps {
		allDeps[k] = v
	}

	for dep := range allDeps {
		for _, native := range knownNativeModules {
			if dep == native || strings.HasPrefix(dep, native+"/") {
				analysis.NativeModules = append(analysis.NativeModules, dep)
				analysis.HasNativeModules = true
			}
		}
	}

	// Also scan node_modules for .node files (compiled binaries)
	nodeModulesDir := filepath.Join(dir, "node_modules")
	if _, err := os.Stat(nodeModulesDir); err == nil {
		nativeFromScan := scanForNativeModules(nodeModulesDir)
		for _, mod := range nativeFromScan {
			// Avoid duplicates
			found := false
			for _, existing := range analysis.NativeModules {
				if existing == mod {
					found = true
					break
				}
			}
			if !found {
				analysis.NativeModules = append(analysis.NativeModules, mod)
				analysis.HasNativeModules = true
			}
		}
	}

	return nil
}

func inferStartCommand(scripts map[string]string) string {
	if start, ok := scripts["start"]; ok {
		return start
	}
	return "node index.js"
}

// scanForNativeModules scans node_modules for .node files or binding.gyp
func scanForNativeModules(nodeModulesDir string) []string {
	var natives []string
	seen := make(map[string]bool)

	filepath.Walk(nodeModulesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip nested node_modules
		rel, _ := filepath.Rel(nodeModulesDir, path)
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) > 2 && parts[1] == "node_modules" {
			return filepath.SkipDir
		}

		// Check for .node files (compiled native modules)
		if strings.HasSuffix(info.Name(), ".node") {
			modName := extractModuleName(nodeModulesDir, path)
			if modName != "" && !seen[modName] {
				natives = append(natives, modName)
				seen[modName] = true
			}
		}

		// Check for binding.gyp (native module build config)
		if info.Name() == "binding.gyp" {
			modName := extractModuleName(nodeModulesDir, path)
			if modName != "" && !seen[modName] {
				natives = append(natives, modName)
				seen[modName] = true
			}
		}

		return nil
	})

	return natives
}

// analyzeStaticSite detects static sites (HTML, Jekyll, Hugo, etc.)
func analyzeStaticSite(dir string, analysis *Analysis) {
	// Jekyll: _config.yml present
	if _, err := os.Stat(filepath.Join(dir, "_config.yml")); err == nil {
		analysis.Framework = "jekyll"
		analysis.IsStaticSite = true
		// If _site/ exists, it's a pre-built Jekyll site
		if _, err := os.Stat(filepath.Join(dir, "_site")); err == nil {
			analysis.OutputDir = "_site"
		} else {
			analysis.OutputDir = "."
		}
		return
	}

	// Hugo: config.toml or hugo.toml present
	for _, cfg := range []string{"hugo.toml", "hugo.yaml", "config.toml"} {
		if _, err := os.Stat(filepath.Join(dir, cfg)); err == nil {
			analysis.Framework = "hugo"
			analysis.IsStaticSite = true
			if _, err := os.Stat(filepath.Join(dir, "public")); err == nil {
				analysis.OutputDir = "public"
			} else {
				analysis.OutputDir = "."
			}
			return
		}
	}

	// Generic static site: index.html in root
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err == nil {
		analysis.Framework = "static"
		analysis.IsStaticSite = true
		analysis.OutputDir = "."
		return
	}
}

func extractModuleName(nodeModulesDir, path string) string {
	rel, err := filepath.Rel(nodeModulesDir, path)
	if err != nil {
		return ""
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 0 {
		return ""
	}
	// Skip pnpm virtual store
	if parts[0] == ".pnpm" {
		return ""
	}
	// Handle scoped packages (@org/package)
	if strings.HasPrefix(parts[0], "@") && len(parts) >= 2 {
		return parts[0] + "/" + parts[1]
	}
	return parts[0]
}

// analyzeGoProject detects Go projects.
func analyzeGoProject(dir string, analysis *Analysis) {
	analysis.Framework = "go"
	analysis.BuildCommand = "go build -o app ."
	analysis.OutputDir = "."
	analysis.StartCommand = "./app"
}

// hasPythonProject checks if directory contains Python project markers.
func hasPythonProject(dir string) bool {
	markers := []string{"pyproject.toml", "requirements.txt", "Pipfile"}
	for _, marker := range markers {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

// analyzePythonProject detects Python framework and configuration.
func analyzePythonProject(dir string, analysis *Analysis) error {
	// Default Python settings
	analysis.Framework = "python"
	analysis.BuildCommand = "pip install -r requirements.txt"
	analysis.OutputDir = "."
	analysis.StartCommand = "python main.py"

	// Detect framework from dependencies
	deps := collectPythonDeps(dir)

	if containsDep(deps, "fastapi") {
		analysis.Framework = "fastapi"
		analysis.StartCommand = "uvicorn main:app --host 0.0.0.0 --port 8080"
	} else if containsDep(deps, "django") {
		analysis.Framework = "django"
		// Try to detect Django project name from manage.py
		projectName := detectDjangoProject(dir)
		if projectName != "" {
			analysis.StartCommand = fmt.Sprintf("gunicorn %s.wsgi:application --bind 0.0.0.0:8080", projectName)
		} else {
			analysis.StartCommand = "gunicorn project.wsgi:application --bind 0.0.0.0:8080"
		}
	} else if containsDep(deps, "flask") {
		analysis.Framework = "flask"
		analysis.StartCommand = "gunicorn app:app --bind 0.0.0.0:8080"
	}

	// Adjust build command based on project type
	if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err == nil {
		data, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
		if err == nil && (strings.Contains(string(data), "[tool.poetry]") || strings.Contains(string(data), "[build-system]")) {
			analysis.BuildCommand = "pip install ."
		}
	}

	return nil
}

// collectPythonDeps extracts dependency names from Python project files.
func collectPythonDeps(dir string) []string {
	var deps []string

	// Check requirements.txt
	if data, err := os.ReadFile(filepath.Join(dir, "requirements.txt")); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			// Extract package name (before ==, >=, etc.)
			parts := strings.FieldsFunc(line, func(r rune) bool {
				return r == '=' || r == '<' || r == '>' || r == '~' || r == '!'
			})
			if len(parts) > 0 {
				deps = append(deps, strings.ToLower(parts[0]))
			}
		}
	}

	// Check pyproject.toml
	if data, err := os.ReadFile(filepath.Join(dir, "pyproject.toml")); err == nil {
		content := string(data)
		// Simple dependency extraction (not a full TOML parser)
		if idx := strings.Index(content, "[tool.poetry.dependencies]"); idx != -1 {
			section := content[idx:]
			if endIdx := strings.Index(section[1:], "\n["); endIdx != -1 {
				section = section[:endIdx+1]
			}
			lines := strings.Split(section, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.Contains(line, "=") && !strings.HasPrefix(line, "[") {
					parts := strings.SplitN(line, "=", 2)
					if len(parts) > 0 {
						dep := strings.TrimSpace(parts[0])
						deps = append(deps, strings.ToLower(dep))
					}
				}
			}
		}
	}

	return deps
}

// containsDep checks if a dependency is in the list (case-insensitive).
func containsDep(deps []string, target string) bool {
	target = strings.ToLower(target)
	for _, dep := range deps {
		if strings.ToLower(dep) == target {
			return true
		}
	}
	return false
}

// detectDjangoProject tries to find Django project name from manage.py or settings.py.
func detectDjangoProject(dir string) string {
	// Try to read manage.py
	if data, err := os.ReadFile(filepath.Join(dir, "manage.py")); err == nil {
		content := string(data)
		// Look for DJANGO_SETTINGS_MODULE pattern
		if idx := strings.Index(content, "DJANGO_SETTINGS_MODULE"); idx != -1 {
			// Extract project name from pattern like "project.settings"
			line := content[idx:]
			if endIdx := strings.Index(line, "\n"); endIdx != -1 {
				line = line[:endIdx]
			}
			// Find quoted string
			if startIdx := strings.Index(line, `"`); startIdx != -1 {
				line = line[startIdx+1:]
				if endIdx := strings.Index(line, `"`); endIdx != -1 {
					settingsModule := line[:endIdx]
					// Extract project name (before .settings)
					parts := strings.Split(settingsModule, ".")
					if len(parts) > 0 {
						return parts[0]
					}
				}
			}
		}
	}
	return ""
}

// analyzeRustProject detects Rust projects.
func analyzeRustProject(dir string, analysis *Analysis) error {
	analysis.Framework = "rust"
	analysis.BuildCommand = "cargo build --release"
	analysis.OutputDir = "target/release"
	analysis.StartCommand = "./app"

	// Try to read Cargo.toml to get the binary name
	if data, err := os.ReadFile(filepath.Join(dir, "Cargo.toml")); err == nil {
		content := string(data)
		// Look for [package] section and name field
		if idx := strings.Index(content, "[package]"); idx != -1 {
			section := content[idx:]
			if endIdx := strings.Index(section[1:], "\n["); endIdx != -1 {
				section = section[:endIdx+1]
			}
			lines := strings.Split(section, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "name") && strings.Contains(line, "=") {
					parts := strings.SplitN(line, "=", 2)
					if len(parts) == 2 {
						name := strings.TrimSpace(parts[1])
						name = strings.Trim(name, `"'`)
						if name != "" {
							analysis.StartCommand = "./" + name
							break
						}
					}
				}
			}
		}
	}

	return nil
}
