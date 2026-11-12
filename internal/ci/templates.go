package ci

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// SupportedRuntimes is the closed list of runtimes `hatch ci` can generate a
// workflow for. Anything else — including a runtime read from .hatch.toml — is
// refused before rendering (h-cmzf.1).
var SupportedRuntimes = []string{"node", "go", "python", "rust", "php", "bun", "static"}

// IsSupportedRuntime reports whether runtime is in SupportedRuntimes.
func IsSupportedRuntime(runtime string) bool {
	for _, r := range SupportedRuntimes {
		if r == runtime {
			return true
		}
	}
	return false
}

// CLIRepo is the GitHub repository whose releases the generated workflow installs.
const CLIRepo = "EscapeVelocityOperations/hatch-cli"

// CLIRelease pins the exact Hatch CLI build the generated workflow downloads:
// a release tag plus the sha256 of each linux binary, checked fail-closed on
// the runner before the binary runs (h-cmzf.5).
type CLIRelease struct {
	Version     string // release tag, e.g. v0.21.0
	SHA256AMD64 string // hex sha256 of hatch-linux-amd64
	SHA256ARM64 string // hex sha256 of hatch-linux-arm64
}

// WorkflowParams are the values rendered into a generated CI deploy workflow.
// Every value reaches `hatch deploy` as data: it is written as an encoded YAML
// scalar in the job environment and passed to the CLI as a quoted "$VAR"
// argument, never spliced into the shell script (h-cmzf.1).
type WorkflowParams struct {
	App          string // Hatch egg slug the workflow deploys to (required)
	Runtime      string // one of SupportedRuntimes (required)
	DeployTarget string // build output dir (required)
	StartCommand string // app start command (required unless runtime is static)
	// GoPackage is the main package the go workflow builds into DeployTarget, as
	// "." or "./cmd/server" (go runtime only; required to render, h-8tvak.4).
	GoPackage string
	CLI       CLIRelease
}

// WorkflowFile is a generated CI config file: its repo-relative path + content.
type WorkflowFile struct {
	Path    string
	Content string
}

var (
	cliVersionRe = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)
	sha256Re     = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// ValidateInputs checks the user-facing parameters (everything but the CLI
// pin) so `hatch ci` can refuse an unusable workflow before doing any work.
func (p WorkflowParams) ValidateInputs() error {
	if !IsSupportedRuntime(p.Runtime) {
		return fmt.Errorf("unsupported runtime %q (use one of: %s)", p.Runtime, strings.Join(SupportedRuntimes, ", "))
	}
	if p.App == "" {
		return fmt.Errorf("no target egg: pass --app <slug> (or run from a directory whose .hatch.toml names the egg)")
	}
	if strings.ContainsFunc(p.App, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		return fmt.Errorf("invalid --app %q: an egg slug cannot contain spaces or control characters", p.App)
	}
	if p.DeployTarget == "" {
		return fmt.Errorf("--deploy-target is required: hatch deploy needs the build output directory")
	}
	if p.Runtime != "static" && p.StartCommand == "" {
		return fmt.Errorf("--start-command is required for runtime %s", p.Runtime)
	}
	for _, v := range []struct{ name, value string }{
		{"--app", p.App}, {"--deploy-target", p.DeployTarget}, {"--start-command", p.StartCommand},
	} {
		if !utf8.ValidString(v.value) || strings.ContainsRune(v.value, 0) {
			return fmt.Errorf("invalid %s: not valid UTF-8 text", v.name)
		}
		// GitHub evaluates ${{ … }} anywhere in a workflow, including env values.
		if strings.Contains(v.value, "${{") {
			return fmt.Errorf("invalid %s: it cannot contain \"${{\" (CI expression syntax)", v.name)
		}
	}
	if p.Runtime == "go" {
		// The binary the build must produce is the one the start command runs.
		if _, err := GoBinaryPath(p.StartCommand); err != nil {
			return err
		}
		if p.GoPackage != "" {
			if err := ValidateGoPackage(p.GoPackage); err != nil {
				return err
			}
		}
	} else if p.GoPackage != "" {
		return fmt.Errorf("--go-package only applies to the go runtime (runtime is %s)", p.Runtime)
	}
	return nil
}

// Validate checks every parameter, including the CLI pin.
func (p WorkflowParams) Validate() error {
	if err := p.ValidateInputs(); err != nil {
		return err
	}
	if p.Runtime == "go" && p.GoPackage == "" {
		return fmt.Errorf("runtime go needs the main package to build: pass --go-package (for example ./cmd/server)")
	}
	if !cliVersionRe.MatchString(p.CLI.Version) {
		return fmt.Errorf("invalid Hatch CLI version %q (want vX.Y.Z)", p.CLI.Version)
	}
	if !sha256Re.MatchString(p.CLI.SHA256AMD64) || !sha256Re.MatchString(p.CLI.SHA256ARM64) {
		return fmt.Errorf("missing or invalid sha256 for Hatch CLI %s linux binaries", p.CLI.Version)
	}
	return nil
}

// RenderWorkflow renders the deploy workflow for the given provider + runtime.
// It returns an error for an unsupported provider or invalid parameters so the
// caller never writes an unusable or injectable workflow.
func RenderWorkflow(provider string, p WorkflowParams) (WorkflowFile, error) {
	if provider != "github" && provider != "gitlab" {
		return WorkflowFile{}, fmt.Errorf("unsupported CI provider %q (use github or gitlab)", provider)
	}
	if err := p.Validate(); err != nil {
		return WorkflowFile{}, err
	}
	if provider == "github" {
		return WorkflowFile{Path: ".github/workflows/hatch-deploy.yml", Content: renderGitHub(p)}, nil
	}
	return WorkflowFile{Path: ".gitlab-ci.yml", Content: renderGitLab(p)}, nil
}

// yamlString encodes s as a YAML double-quoted scalar. A JSON string is a valid
// YAML 1.2 double-quoted scalar, so quotes, backslashes, newlines, ": " and "#"
// all stay data. Callers validate s as UTF-8 first.
func yamlString(s string) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // cannot fail for a string
	return strings.TrimSuffix(b.String(), "\n")
}

// envVar is one job environment variable of the deploy step.
type envVar struct{ name, value string }

// deployEnv lists the variables the deploy script reads. HATCH_TOKEN is not in
// it: it comes from the provider's secret store.
func deployEnv(p WorkflowParams) []envVar {
	env := []envVar{
		// The account owner accepted the Terms of Service to run `hatch ci`;
		// this records that acceptance for the CLI on the stateless runner (h-cmzf.4).
		{"HATCH_ACCEPT_TERMS", "yes"},
		{"HATCH_CI_CLI_VERSION", p.CLI.Version},
		{"HATCH_CI_CLI_SHA256_AMD64", p.CLI.SHA256AMD64},
		{"HATCH_CI_CLI_SHA256_ARM64", p.CLI.SHA256ARM64},
		{"HATCH_CI_APP", p.App},
		{"HATCH_CI_RUNTIME", p.Runtime},
		{"HATCH_CI_DEPLOY_TARGET", p.DeployTarget},
	}
	if p.StartCommand != "" {
		env = append(env, envVar{"HATCH_CI_START_COMMAND", p.StartCommand})
	}
	return env
}

// goBuildEnv lists the variables the go build step reads: the package to build
// and where to write the binary (inside the deploy-target). Only go has any.
// Both values are validated by ValidateInputs/Validate before rendering.
func goBuildEnv(p WorkflowParams) []envVar {
	if p.Runtime != "go" {
		return nil
	}
	out, _ := GoBuildOutput(p) // the start command was validated
	return []envVar{
		{"HATCH_CI_GO_PACKAGE", p.GoPackage},
		{"HATCH_CI_GO_OUTPUT", out},
	}
}

// goBuildScript compiles the chosen main package to a static linux/amd64 binary
// at the output path (go build -o creates the directories). Like the deploy
// script it contains no parameter value, only quoted "$VAR" references.
const goBuildScript = `set -eu
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$HATCH_CI_GO_OUTPUT" "$HATCH_CI_GO_PACKAGE"
`

// deployScript is the POSIX sh script that installs the pinned CLI (download,
// fail-closed sha256 check) and deploys. It contains no parameter value: every
// value is read from the environment as a quoted "$VAR" argument.
func deployScript(p WorkflowParams) string {
	deploy := `"$hatch_dir/hatch" deploy --app "$HATCH_CI_APP" --runtime "$HATCH_CI_RUNTIME" --deploy-target "$HATCH_CI_DEPLOY_TARGET"`
	if p.StartCommand != "" {
		deploy += ` --start-command "$HATCH_CI_START_COMMAND"`
	}
	return `set -eu
case "$(uname -m)" in
  x86_64|amd64) hatch_arch=amd64; hatch_sha="$HATCH_CI_CLI_SHA256_AMD64" ;;
  aarch64|arm64) hatch_arch=arm64; hatch_sha="$HATCH_CI_CLI_SHA256_ARM64" ;;
  *) echo "hatch: unsupported runner architecture $(uname -m)" >&2; exit 1 ;;
esac
hatch_dir="$(mktemp -d)"
hatch_url="https://github.com/` + CLIRepo + `/releases/download/$HATCH_CI_CLI_VERSION/hatch-linux-$hatch_arch"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL -o "$hatch_dir/hatch" "$hatch_url"
else
  wget -q -O "$hatch_dir/hatch" "$hatch_url"
fi
hatch_got="$(sha256sum "$hatch_dir/hatch" | cut -d ' ' -f 1)"
if [ "$hatch_got" != "$hatch_sha" ]; then
  echo "hatch: checksum mismatch for $hatch_url (got $hatch_got, want $hatch_sha)" >&2
  exit 1
fi
chmod +x "$hatch_dir/hatch"
` + deploy + `
`
}

// indent prefixes every non-empty line of s with n spaces.
func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = pad + l
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// GitHub actions pinned to full commit SHAs (h-cmzf.5). Verified 2026-11-10 with
// `gh api repos/<owner>/<repo>/commits/<tag>`; bump them together, deliberately.
const (
	actionCheckout    = "actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1"
	actionSetupNode   = "actions/setup-node@949feb2413d6458794dcd2491c4babbbce0c15c1 # v7.1.0"
	actionSetupGo     = "actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0"
	actionSetupPython = "actions/setup-python@6d4fd22b20465ccc0f7354476588e8df7c9ca0e0 # v7.1.0"
	actionSetupBun    = "oven-sh/setup-bun@0c5077e51419868618aeaa5fe8019c62421857d6 # v2.2.0"
	actionSetupPHP    = "shivammathur/setup-php@eb7c497e18156a6bbabfef1d3a82760b9eda3962 # 2.40.0"
	actionRust        = "dtolnay/rust-toolchain@b725eda525362e7cf34af9a7243cac1e3f01a7ec # stable branch"
)

func renderGitHub(p WorkflowParams) string {
	env := "          HATCH_TOKEN: ${{ secrets.HATCH_TOKEN }}\n" + githubEnvLines(deployEnv(p))
	return fmt.Sprintf(`# Generated by hatch ci. Review it before committing.
# Deploy parameters are data in the env block below; the script never embeds them.
name: Deploy to Hatch

on:
  push:
    branches: [main]

permissions:
  contents: read

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: %s
        with:
          persist-credentials: false
%s      - name: Install Hatch CLI %s and deploy
        shell: sh
        env:
%s        run: |
%s`, actionCheckout, githubBuildSteps(p), p.CLI.Version, env, indent(deployScript(p), 10))
}

// githubEnvLines renders vars as env entries of a step (10-space indent).
func githubEnvLines(vars []envVar) string {
	var b strings.Builder
	for _, v := range vars {
		fmt.Fprintf(&b, "          %s: %s\n", v.name, yamlString(v.value))
	}
	return b.String()
}

// githubBuildSteps returns the runtime toolchain-setup + build steps (YAML, at
// 6-space indent to sit under `steps:`), or "" for a static site (nothing to build).
func githubBuildSteps(p WorkflowParams) string {
	switch p.Runtime {
	case "node":
		return "      - uses: " + actionSetupNode + "\n" +
			"        with:\n          node-version: '20'\n" +
			"      - run: npm ci\n" +
			"      - run: npm run build --if-present\n"
	case "bun":
		return "      - uses: " + actionSetupBun + "\n" +
			"      - run: bun install\n" +
			"      - run: bun run build\n"
	case "go":
		return "      - uses: " + actionSetupGo + "\n" +
			"        with:\n          go-version: '1.22'\n" +
			"      - name: Build the linux/amd64 binary Hatch deploys\n" +
			"        shell: sh\n" +
			"        env:\n" + githubEnvLines(goBuildEnv(p)) +
			"        run: |\n" + indent(goBuildScript, 10)
	case "python":
		return "      - uses: " + actionSetupPython + "\n" +
			"        with:\n          python-version: '3.12'\n" +
			"      - run: pip install -r requirements.txt\n"
	case "rust":
		// Pinned to a SHA, the action can no longer infer the toolchain from its ref.
		return "      - uses: " + actionRust + "\n" +
			"        with:\n          toolchain: stable\n" +
			"      - run: cargo build --release\n"
	case "php":
		return "      - uses: " + actionSetupPHP + "\n" +
			"        with:\n          php-version: '8.3'\n" +
			"      - run: composer install --no-dev\n"
	default: // static — nothing to build
		return ""
	}
}

func renderGitLab(p WorkflowParams) string {
	var vars strings.Builder
	for _, v := range append(deployEnv(p), goBuildEnv(p)...) {
		fmt.Fprintf(&vars, "    %s:\n      value: %s\n      expand: false\n", v.name, yamlString(v.value))
	}
	return fmt.Sprintf(`# Generated by hatch ci. Review it before committing.
# HATCH_TOKEN comes from a masked CI/CD variable. Deploy parameters are data in
# the variables block below (expand: false); the script never embeds them.
stages:
  - deploy

deploy:
  stage: deploy
  image: %s
  only:
    - main
  variables:
%s  script:
%s    - |
%s`, gitlabImage(p.Runtime), vars.String(), gitlabBuildScript(p), indent(deployScript(p), 6))
}

// gitlabImage picks a base image for the runtime's build, pinned to the
// multi-arch index digest (h-cmzf.5). Verified 2026-11-10 against Docker Hub
// (registry-1.docker.io manifests, Docker-Content-Digest).
func gitlabImage(runtime string) string {
	switch runtime {
	case "node":
		return "node:20@sha256:8f693eaa7e0a8e71560c9a82b55fd54c2ae920a2ba5d2cde28bac7d1c01c9ba5"
	case "bun":
		return "oven/bun:1@sha256:1d22548f177f4ff38e0c15baa3c2c46c6c2060ba2d1bc573bb4720b407548f1d"
	case "go":
		return "golang:1.22@sha256:1cf6c45ba39db9fd6db16922041d074a63c935556a05c5ccb62d181034df7f02"
	case "python":
		return "python:3.12@sha256:cad37dd20c7cac861af9003af8796d03ea7a998ec1e3a664429740fc9d7d2691"
	case "rust":
		return "rust:1@sha256:8a4e90c7946b74626488b1446f712c0fe0ed1a596cbd3d871684bc9c5433ae07"
	case "php":
		return "php:8.3-cli@sha256:628102249d93494233b54490b785f12db9cd60f4dc89416dfca15973f2125f10"
	default: // static
		return "alpine:3@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf"
	}
}

// gitlabBuildScript returns the build script lines (4-space indent under `script:`),
// or "" for a static site.
func gitlabBuildScript(p WorkflowParams) string {
	switch p.Runtime {
	case "node":
		return "    - npm ci\n    - npm run build --if-present\n"
	case "bun":
		return "    - bun install\n    - bun run build\n"
	case "go":
		return "    - |\n" + indent(goBuildScript, 6)
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
