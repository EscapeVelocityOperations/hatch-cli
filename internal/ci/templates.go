package ci

import "fmt"

// WorkflowParams are the values rendered into a generated CI deploy workflow.
type WorkflowParams struct {
	Runtime      string // node, go, python, rust, php, bun, static
	DeployTarget string // build output dir (optional)
	StartCommand string // app start command (optional)
}

// WorkflowFile is a generated CI config file: its repo-relative path + content.
type WorkflowFile struct {
	Path    string
	Content string
}

// RenderWorkflow renders the deploy workflow for the given provider + runtime.
// It returns an error for an unsupported provider so the caller can guide the user.
func RenderWorkflow(provider string, p WorkflowParams) (WorkflowFile, error) {
	switch provider {
	case "github":
		return WorkflowFile{Path: ".github/workflows/hatch-deploy.yml", Content: renderGitHub(p)}, nil
	case "gitlab":
		return WorkflowFile{Path: ".gitlab-ci.yml", Content: renderGitLab(p)}, nil
	default:
		return WorkflowFile{}, fmt.Errorf("unsupported CI provider %q (use github or gitlab)", provider)
	}
}

// deployCmd builds the `hatch deploy` invocation, omitting optional flags when
// unset and wiring the token to the provider-specific secret reference.
func deployCmd(p WorkflowParams, tokenRef string) string {
	cmd := "hatch deploy --runtime " + p.Runtime
	if p.DeployTarget != "" {
		cmd += " --deploy-target " + p.DeployTarget
	}
	if p.StartCommand != "" {
		cmd += ` --start-command "` + p.StartCommand + `"`
	}
	return cmd + " --token " + tokenRef
}

func renderGitHub(p WorkflowParams) string {
	return fmt.Sprintf(`name: Deploy to Hatch

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
%s      - name: Install Hatch CLI
        run: curl -fsSL https://gethatch.eu/install | sh
      - name: Deploy to Hatch
        run: %s
`, githubBuildSteps(p.Runtime), deployCmd(p, "${{ secrets.HATCH_TOKEN }}"))
}

// githubBuildSteps returns the runtime toolchain-setup + build steps (YAML, at
// 6-space indent to sit under `steps:`), or "" for a static site (nothing to build).
func githubBuildSteps(runtime string) string {
	switch runtime {
	case "node":
		return "      - uses: actions/setup-node@v4\n" +
			"        with:\n          node-version: '20'\n" +
			"      - run: npm ci\n" +
			"      - run: npm run build --if-present\n"
	case "bun":
		return "      - uses: oven-sh/setup-bun@v2\n" +
			"      - run: bun install\n" +
			"      - run: bun run build\n"
	case "go":
		return "      - uses: actions/setup-go@v5\n" +
			"        with:\n          go-version: '1.22'\n" +
			"      - run: go build ./...\n"
	case "python":
		return "      - uses: actions/setup-python@v5\n" +
			"        with:\n          python-version: '3.12'\n" +
			"      - run: pip install -r requirements.txt\n"
	case "rust":
		return "      - uses: dtolnay/rust-toolchain@stable\n" +
			"      - run: cargo build --release\n"
	case "php":
		return "      - uses: shivammathur/setup-php@v2\n" +
			"        with:\n          php-version: '8.3'\n" +
			"      - run: composer install --no-dev\n"
	default: // static — nothing to build
		return ""
	}
}

func renderGitLab(p WorkflowParams) string {
	return fmt.Sprintf(`stages:
  - build
  - deploy

deploy:
  stage: deploy
  image: %s
  only:
    - main
  script:
%s    - curl -fsSL https://gethatch.eu/install | sh
    - %s
`, gitlabImage(p.Runtime), gitlabBuildScript(p.Runtime), deployCmd(p, "$HATCH_TOKEN"))
}

// gitlabImage picks a base image for the runtime's build.
func gitlabImage(runtime string) string {
	switch runtime {
	case "node":
		return "node:20"
	case "bun":
		return "oven/bun:1"
	case "go":
		return "golang:1.22"
	case "python":
		return "python:3.12"
	case "rust":
		return "rust:latest"
	case "php":
		return "php:8.3-cli"
	default: // static
		return "alpine:latest"
	}
}

// gitlabBuildScript returns the build script lines (4-space indent under `script:`),
// or "" for a static site.
func gitlabBuildScript(runtime string) string {
	switch runtime {
	case "node":
		return "    - npm ci\n    - npm run build --if-present\n"
	case "bun":
		return "    - bun install\n    - bun run build\n"
	case "go":
		return "    - go build ./...\n"
	case "python":
		return "    - pip install -r requirements.txt\n"
	case "rust":
		return "    - cargo build --release\n"
	case "php":
		return "    - composer install --no-dev\n"
	default: // static
		return ""
	}
}
