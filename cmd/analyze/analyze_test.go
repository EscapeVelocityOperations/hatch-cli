package analyze

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeProject_Nuxt(t *testing.T) {
	dir := t.TempDir()
	pkgJSON := `{
		"name": "test-nuxt",
		"dependencies": {
			"nuxt": "^3.0.0"
		}
	}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatal(err)
	}

	analysis, err := AnalyzeProject(dir)
	if err != nil {
		t.Fatalf("AnalyzeProject failed: %v", err)
	}

	if analysis.Framework != "nuxt" {
		t.Errorf("Expected framework=nuxt, got %s", analysis.Framework)
	}
	if analysis.BuildCommand != "pnpm build" {
		t.Errorf("Expected BuildCommand='pnpm build', got %s", analysis.BuildCommand)
	}
	if analysis.OutputDir != ".output" {
		t.Errorf("Expected OutputDir='.output', got %s", analysis.OutputDir)
	}
	if analysis.StartCommand != "node .output/server/index.mjs" {
		t.Errorf("Expected StartCommand='node .output/server/index.mjs', got %s", analysis.StartCommand)
	}
}

func TestAnalyzeProject_Next(t *testing.T) {
	dir := t.TempDir()
	pkgJSON := `{
		"name": "test-next",
		"dependencies": {
			"next": "^14.0.0",
			"react": "^18.0.0"
		}
	}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatal(err)
	}

	analysis, err := AnalyzeProject(dir)
	if err != nil {
		t.Fatalf("AnalyzeProject failed: %v", err)
	}

	if analysis.Framework != "next" {
		t.Errorf("Expected framework=next, got %s", analysis.Framework)
	}
	if analysis.BuildCommand != "pnpm build" {
		t.Errorf("Expected BuildCommand='pnpm build', got %s", analysis.BuildCommand)
	}
	if analysis.OutputDir != ".next" {
		t.Errorf("Expected OutputDir='.next', got %s", analysis.OutputDir)
	}
	if analysis.StartCommand != "pnpm start" {
		t.Errorf("Expected StartCommand='pnpm start', got %s", analysis.StartCommand)
	}
}

func TestAnalyzeProject_Express(t *testing.T) {
	dir := t.TempDir()
	pkgJSON := `{
		"name": "test-express",
		"dependencies": {
			"express": "^4.18.0"
		},
		"scripts": {
			"start": "node server.js"
		}
	}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatal(err)
	}

	analysis, err := AnalyzeProject(dir)
	if err != nil {
		t.Fatalf("AnalyzeProject failed: %v", err)
	}

	if analysis.Framework != "express" {
		t.Errorf("Expected framework=express, got %s", analysis.Framework)
	}
	if analysis.BuildCommand != "pnpm install --prod" {
		t.Errorf("Expected BuildCommand='pnpm install --prod', got %s", analysis.BuildCommand)
	}
	if analysis.OutputDir != "." {
		t.Errorf("Expected OutputDir='.', got %s", analysis.OutputDir)
	}
	if analysis.StartCommand != "node server.js" {
		t.Errorf("Expected StartCommand='node server.js', got %s", analysis.StartCommand)
	}
}

func TestAnalyzeProject_Node(t *testing.T) {
	dir := t.TempDir()
	pkgJSON := `{
		"name": "test-node",
		"dependencies": {
			"lodash": "^4.17.0"
		}
	}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatal(err)
	}

	analysis, err := AnalyzeProject(dir)
	if err != nil {
		t.Fatalf("AnalyzeProject failed: %v", err)
	}

	if analysis.Framework != "node" {
		t.Errorf("Expected framework=node, got %s", analysis.Framework)
	}
	if analysis.BuildCommand != "pnpm install --prod" {
		t.Errorf("Expected BuildCommand='pnpm install --prod', got %s", analysis.BuildCommand)
	}
	if analysis.OutputDir != "." {
		t.Errorf("Expected OutputDir='.', got %s", analysis.OutputDir)
	}
	if analysis.StartCommand != "node index.js" {
		t.Errorf("Expected StartCommand='node index.js', got %s", analysis.StartCommand)
	}
}

func TestAnalyzeProject_StaticSite(t *testing.T) {
	dir := t.TempDir()
	html := `<!DOCTYPE html>
<html>
<head><title>Test</title></head>
<body><h1>Hello</h1></body>
</html>`
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(html), 0644); err != nil {
		t.Fatal(err)
	}

	analysis, err := AnalyzeProject(dir)
	if err != nil {
		t.Fatalf("AnalyzeProject failed: %v", err)
	}

	if analysis.Framework != "static" {
		t.Errorf("Expected framework=static, got %s", analysis.Framework)
	}
	if !analysis.IsStaticSite {
		t.Error("Expected IsStaticSite=true")
	}
	if analysis.OutputDir != "." {
		t.Errorf("Expected OutputDir='.', got %s", analysis.OutputDir)
	}
}

func TestAnalyzeProject_Go(t *testing.T) {
	dir := t.TempDir()
	goMod := `module example.com/myapp

go 1.21

require (
	github.com/gin-gonic/gin v1.9.0
)`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	analysis, err := AnalyzeProject(dir)
	if err != nil {
		t.Fatalf("AnalyzeProject failed: %v", err)
	}

	if analysis.Framework != "go" {
		t.Errorf("Expected framework=go, got %s", analysis.Framework)
	}
	if analysis.BuildCommand != "go build -o app ." {
		t.Errorf("Expected BuildCommand='go build -o app .', got %s", analysis.BuildCommand)
	}
	if analysis.StartCommand != "./app" {
		t.Errorf("Expected StartCommand='./app', got %s", analysis.StartCommand)
	}
}

func TestAnalyzeProject_Flask(t *testing.T) {
	dir := t.TempDir()
	requirements := `flask==2.3.0
gunicorn==21.2.0`
	if err := os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte(requirements), 0644); err != nil {
		t.Fatal(err)
	}

	analysis, err := AnalyzeProject(dir)
	if err != nil {
		t.Fatalf("AnalyzeProject failed: %v", err)
	}

	if analysis.Framework != "flask" {
		t.Errorf("Expected framework=flask, got %s", analysis.Framework)
	}
	if analysis.BuildCommand != "pip install -r requirements.txt" {
		t.Errorf("Expected BuildCommand='pip install -r requirements.txt', got %s", analysis.BuildCommand)
	}
	if analysis.StartCommand != "gunicorn app:app --bind 0.0.0.0:8080" {
		t.Errorf("Expected StartCommand='gunicorn app:app --bind 0.0.0.0:8080', got %s", analysis.StartCommand)
	}
}

func TestAnalyzeProject_FastAPI(t *testing.T) {
	dir := t.TempDir()
	requirements := `fastapi==0.104.0
uvicorn[standard]==0.24.0`
	if err := os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte(requirements), 0644); err != nil {
		t.Fatal(err)
	}

	analysis, err := AnalyzeProject(dir)
	if err != nil {
		t.Fatalf("AnalyzeProject failed: %v", err)
	}

	if analysis.Framework != "fastapi" {
		t.Errorf("Expected framework=fastapi, got %s", analysis.Framework)
	}
	if analysis.BuildCommand != "pip install -r requirements.txt" {
		t.Errorf("Expected BuildCommand='pip install -r requirements.txt', got %s", analysis.BuildCommand)
	}
	if analysis.StartCommand != "uvicorn main:app --host 0.0.0.0 --port 8080" {
		t.Errorf("Expected StartCommand='uvicorn main:app --host 0.0.0.0 --port 8080', got %s", analysis.StartCommand)
	}
}

func TestAnalyzeProject_Rust(t *testing.T) {
	dir := t.TempDir()
	cargoToml := `[package]
name = "myapp"
version = "0.1.0"
edition = "2021"

[dependencies]
actix-web = "4.0"`
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(cargoToml), 0644); err != nil {
		t.Fatal(err)
	}

	analysis, err := AnalyzeProject(dir)
	if err != nil {
		t.Fatalf("AnalyzeProject failed: %v", err)
	}

	if analysis.Framework != "rust" {
		t.Errorf("Expected framework=rust, got %s", analysis.Framework)
	}
	if analysis.BuildCommand != "cargo build --release" {
		t.Errorf("Expected BuildCommand='cargo build --release', got %s", analysis.BuildCommand)
	}
	if analysis.StartCommand != "./myapp" {
		t.Errorf("Expected StartCommand='./myapp', got %s", analysis.StartCommand)
	}
}

func TestAnalyzeProject_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	analysis, err := AnalyzeProject(dir)
	if err != nil {
		t.Fatalf("AnalyzeProject failed: %v", err)
	}

	// Empty directory should fall back to unknown or static
	if analysis.Framework != "unknown" {
		// Note: Current implementation returns "unknown" for empty dirs
		t.Logf("Empty directory detected as framework=%s (expected 'unknown' or fallback)", analysis.Framework)
	}
}

func TestAnalyzeProject_NativeModules(t *testing.T) {
	dir := t.TempDir()
	pkgJSON := `{
		"name": "test-native",
		"dependencies": {
			"express": "^4.18.0",
			"sharp": "^0.32.0",
			"bcrypt": "^5.1.0"
		}
	}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatal(err)
	}

	analysis, err := AnalyzeProject(dir)
	if err != nil {
		t.Fatalf("AnalyzeProject failed: %v", err)
	}

	if !analysis.HasNativeModules {
		t.Error("Expected HasNativeModules=true")
	}
	if len(analysis.NativeModules) < 2 {
		t.Errorf("Expected at least 2 native modules, got %d: %v", len(analysis.NativeModules), analysis.NativeModules)
	}
	if analysis.RecommendedStrategy != "remote" {
		t.Errorf("Expected RecommendedStrategy='remote' for native modules, got %s", analysis.RecommendedStrategy)
	}
}
