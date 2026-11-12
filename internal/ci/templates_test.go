package ci

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/ci/workflowtest"
	"gopkg.in/yaml.v3"
)

var testCLI = CLIRelease{
	Version:     "v0.21.0",
	SHA256AMD64: workflowtest.FakeHatchSHA256(),
	SHA256ARM64: workflowtest.FakeHatchSHA256(),
}

func validParams(runtime string) WorkflowParams {
	p := WorkflowParams{App: "fixture-stage", Runtime: runtime, DeployTarget: "dist", StartCommand: "node server.js", CLI: testCLI}
	if runtime == "static" {
		p.StartCommand = ""
	}
	if runtime == "python" {
		p.PythonManifest = PythonRequirements
	}
	return p
}

var (
	usesRe  = regexp.MustCompile(`(?m)^\s*- uses: (\S+)`)
	imageRe = regexp.MustCompile(`(?m)^\s*image: (\S+)`)
)

// h-tymh/h-e2h5 + h-cmzf.1/.5: every provider × runtime renders valid YAML with
// the push→main trigger, the toolchain setup, the pinned CLI install, and only
// immutable references (action SHAs, image digests, no remote installer).
func TestRenderWorkflow_AllProvidersAndRuntimes(t *testing.T) {
	setup := map[string]string{
		"node": "actions/setup-node@", "bun": "oven-sh/setup-bun@", "go": "actions/setup-go@",
		"python": "actions/setup-python@", "rust": "dtolnay/rust-toolchain@", "php": "shivammathur/setup-php@",
	}
	for _, provider := range []string{"github", "gitlab"} {
		for _, rt := range SupportedRuntimes {
			t.Run(provider+"/"+rt, func(t *testing.T) {
				wf, err := RenderWorkflow(provider, validParams(rt))
				if err != nil {
					t.Fatal(err)
				}
				var doc map[string]any
				if err := yaml.Unmarshal([]byte(wf.Content), &doc); err != nil {
					t.Fatalf("invalid YAML: %v\n%s", err, wf.Content)
				}
				for _, bad := range []string{"| sh", "gethatch.eu/install", ":latest", "@v", "@stable"} {
					if strings.Contains(wf.Content, bad) {
						t.Errorf("mutable reference %q in workflow:\n%s", bad, wf.Content)
					}
				}
				if !strings.Contains(wf.Content, "releases/download/$HATCH_CI_CLI_VERSION/hatch-linux-$hatch_arch") {
					t.Errorf("workflow does not install the pinned CLI release:\n%s", wf.Content)
				}
				switch provider {
				case "github":
					if wf.Path != ".github/workflows/hatch-deploy.yml" {
						t.Errorf("path = %q", wf.Path)
					}
					for _, want := range []string{"branches: [main]", "contents: read", "HATCH_TOKEN: ${{ secrets.HATCH_TOKEN }}"} {
						if !strings.Contains(wf.Content, want) {
							t.Errorf("missing %q:\n%s", want, wf.Content)
						}
					}
					uses := usesRe.FindAllStringSubmatch(wf.Content, -1)
					if len(uses) == 0 {
						t.Fatal("no actions found")
					}
					for _, u := range uses {
						if !regexp.MustCompile(`^[\w.-]+/[\w.-]+@[0-9a-f]{40}$`).MatchString(u[1]) {
							t.Errorf("action %q is not pinned to a full commit SHA", u[1])
						}
					}
					if s, ok := setup[rt]; ok && !strings.Contains(wf.Content, s) {
						t.Errorf("missing toolchain setup %q", s)
					}
					if rt == "rust" && !strings.Contains(wf.Content, "toolchain: stable") {
						t.Error("SHA-pinned rust-toolchain needs an explicit toolchain input")
					}
				case "gitlab":
					if wf.Path != ".gitlab-ci.yml" {
						t.Errorf("path = %q", wf.Path)
					}
					if strings.Contains(wf.Content, "secrets.") || strings.Contains(wf.Content, "${{") {
						t.Error("gitlab workflow must not carry GitHub expressions")
					}
					img := imageRe.FindStringSubmatch(wf.Content)
					if img == nil || !regexp.MustCompile(`^[\w./-]+:[\w.-]+@sha256:[0-9a-f]{64}$`).MatchString(img[1]) {
						t.Errorf("image %v is not pinned to a digest", img)
					}
				}
			})
		}
	}
}

func TestRenderWorkflow_UnknownProvider(t *testing.T) {
	if _, err := RenderWorkflow("bitbucket", validParams("node")); err == nil {
		t.Fatal("expected an error for an unsupported provider")
	}
}

// h-cmzf.1/.3/.5: unusable or unsafe parameters are refused, never rendered.
func TestRenderWorkflow_RefusesInvalidParams(t *testing.T) {
	cases := map[string]func(p *WorkflowParams){
		"runtime outside the closed list":  func(p *WorkflowParams) { p.Runtime = "node; printf injected" },
		"empty runtime":                    func(p *WorkflowParams) { p.Runtime = "" },
		"no app":                           func(p *WorkflowParams) { p.App = "" },
		"app with a space":                 func(p *WorkflowParams) { p.App = "fixture stage" },
		"app with a newline":               func(p *WorkflowParams) { p.App = "a\nb" },
		"no deploy target":                 func(p *WorkflowParams) { p.DeployTarget = "" },
		"non-static without start command": func(p *WorkflowParams) { p.StartCommand = "" },
		"github expression":                func(p *WorkflowParams) { p.StartCommand = "echo ${{ secrets.HATCH_TOKEN }}" },
		"invalid UTF-8":                    func(p *WorkflowParams) { p.DeployTarget = "d\xffist" },
		"NUL byte":                         func(p *WorkflowParams) { p.StartCommand = "a\x00b" },
		"CLI version not a tag":            func(p *WorkflowParams) { p.CLI.Version = "dev" },
		"CLI sha missing":                  func(p *WorkflowParams) { p.CLI.SHA256ARM64 = "" },
		"CLI sha not hex":                  func(p *WorkflowParams) { p.CLI.SHA256AMD64 = strings.Repeat("z", 64) },
	}
	for name, mut := range cases {
		for _, provider := range []string{"github", "gitlab"} {
			t.Run(provider+"/"+name, func(t *testing.T) {
				p := validParams("node")
				mut(&p)
				if wf, err := RenderWorkflow(provider, p); err == nil {
					t.Fatalf("expected refusal, got workflow:\n%s", wf.Content)
				}
			})
		}
	}
	// A static site needs no start command.
	if _, err := RenderWorkflow("github", validParams("static")); err != nil {
		t.Fatalf("static without start command must render: %v", err)
	}
}

// hostile values reach `hatch deploy` byte-for-byte as single arguments after
// the provider's YAML parsing AND the runner shell (h-cmzf.1).
var hostile = []struct{ name, value string }{
	{"dollar var", "node server.js --port $PORT"},
	{"command substitution", "node $(printf vigie-executed) `printf vigie-backtick`"},
	{"quotes", `it's "quoted" \ and \"escaped\"`},
	{"yaml mapping", "a: b # not a comment"},
	{"newline", "first line\nsecond: line"},
	{"semicolon", "node app.js; printf injected"},
	{"leading dash and glob", "-x * ?"},
	{"unicode", "héllo — ✓"},
	{"gitlab var syntax", "$CI_JOB_TOKEN ${HATCH_TOKEN} $$"},
}

func TestWorkflow_ArgvPreservedThroughYAMLAndShell(t *testing.T) {
	for _, provider := range []string{"github", "gitlab"} {
		for _, h := range hostile {
			t.Run(provider+"/"+h.name, func(t *testing.T) {
				p := validParams("node")
				p.StartCommand = h.value
				p.DeployTarget = "my dist " + h.value
				wf, err := RenderWorkflow(provider, p)
				if err != nil {
					t.Fatal(err)
				}
				d, err := workflowtest.Extract(provider, wf.Content)
				if err != nil {
					t.Fatalf("%v\n%s", err, wf.Content)
				}
				if provider == "gitlab" {
					for k := range d.Env {
						if !d.NoExpand[k] {
							t.Errorf("gitlab variable %s must be expand: false", k)
						}
					}
				}
				if strings.Contains(d.Script, h.value) {
					t.Errorf("parameter value spliced into the shell script:\n%s", d.Script)
				}
				work := t.TempDir()
				res, err := workflowtest.Run(d, work, workflowtest.FakeHatch, map[string]string{
					"PORT": "49123", "HATCH_TOKEN": "fake-token", "CI_JOB_TOKEN": "job-token",
				})
				if err != nil {
					t.Fatal(err)
				}
				if res.Err != nil {
					t.Fatalf("script failed: %v\n%s", res.Err, res.Output)
				}
				want := []string{"deploy", "--app", "fixture-stage", "--runtime", "node",
					"--deploy-target", "my dist " + h.value, "--start-command", h.value}
				if !reflect.DeepEqual(res.Argv, want) {
					t.Errorf("argv = %q\nwant  %q", res.Argv, want)
				}
				if want := "https://github.com/" + CLIRepo + "/releases/download/v0.21.0/hatch-linux-"; !strings.HasPrefix(res.URL, want) {
					t.Errorf("downloaded %q, want prefix %q", res.URL, want)
				}
				if strings.Contains(res.Output, "vigie-") || strings.Contains(res.Output, "injected") {
					t.Errorf("a parameter was executed by the shell:\n%s", res.Output)
				}
				if d.Env["HATCH_ACCEPT_TERMS"] != "yes" {
					t.Errorf("HATCH_ACCEPT_TERMS = %q, want yes", d.Env["HATCH_ACCEPT_TERMS"])
				}
			})
		}
	}
}

// Static sites deploy without --start-command.
func TestWorkflow_StaticOmitsStartCommand(t *testing.T) {
	for _, provider := range []string{"github", "gitlab"} {
		wf, err := RenderWorkflow(provider, validParams("static"))
		if err != nil {
			t.Fatal(err)
		}
		d, err := workflowtest.Extract(provider, wf.Content)
		if err != nil {
			t.Fatal(err)
		}
		res, err := workflowtest.Run(d, t.TempDir(), workflowtest.FakeHatch, nil)
		if err != nil || res.Err != nil {
			t.Fatalf("%v %v\n%s", err, res.Err, res.Output)
		}
		want := []string{"deploy", "--app", "fixture-stage", "--runtime", "static", "--deploy-target", "dist"}
		if !reflect.DeepEqual(res.Argv, want) {
			t.Errorf("%s: argv = %q, want %q", provider, res.Argv, want)
		}
	}
}

// h-cmzf.5: a downloaded CLI whose bytes differ from the pinned sha256 never runs.
func TestWorkflow_TamperedCLIFailsClosed(t *testing.T) {
	for _, provider := range []string{"github", "gitlab"} {
		wf, err := RenderWorkflow(provider, validParams("node"))
		if err != nil {
			t.Fatal(err)
		}
		d, err := workflowtest.Extract(provider, wf.Content)
		if err != nil {
			t.Fatal(err)
		}
		tampered := workflowtest.FakeHatch + "printf tampered\n"
		res, err := workflowtest.Run(d, t.TempDir(), tampered, nil)
		if err != nil {
			t.Fatal(err)
		}
		if res.Err == nil {
			t.Fatalf("%s: tampered binary accepted:\n%s", provider, res.Output)
		}
		if res.Argv != nil {
			t.Errorf("%s: tampered binary was executed with %q", provider, res.Argv)
		}
		if !strings.Contains(res.Output, "checksum mismatch") {
			t.Errorf("%s: want a checksum mismatch error, got:\n%s", provider, res.Output)
		}
	}
}

func TestParseChecksums(t *testing.T) {
	a, b := strings.Repeat("a", 64), strings.Repeat("B", 64)
	good := a + "  hatch-darwin-arm64\n" + a + "  hatch-linux-amd64\n" + b + " *hatch-linux-arm64\n"
	rel, err := ParseChecksums("v0.21.0", strings.NewReader(good))
	if err != nil {
		t.Fatal(err)
	}
	if rel.SHA256AMD64 != a || rel.SHA256ARM64 != strings.ToLower(b) || rel.Version != "v0.21.0" {
		t.Errorf("rel = %+v", rel)
	}
	for name, in := range map[string]string{
		"missing arm64": a + "  hatch-linux-amd64\n",
		"short sum":     "abc  hatch-linux-amd64\nabc  hatch-linux-arm64\n",
		"empty":         "",
	} {
		if _, err := ParseChecksums("v0.21.0", strings.NewReader(in)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestCheckCIVersion(t *testing.T) {
	for v, ok := range map[string]bool{
		"v0.21.0": true, "v0.22.3": true, "v1.0.0": true, "v0.21.10": true,
		"v0.20.0": false, "v0.9.9": false, "dev": false, "0.21.0": false, "v0.21": false, "": false,
	} {
		if err := CheckCIVersion(v); (err == nil) != ok {
			t.Errorf("CheckCIVersion(%q) err = %v, want ok=%v", v, err, ok)
		}
	}
}

func TestAppSlugFromHatchToml(t *testing.T) {
	for name, tc := range map[string]struct{ content, want string }{
		"app section": {"[app]\nslug = \"fixture-prod\"\nname = \"x\"\n", "fixture-prod"},
		"flat":        {"slug = \"flat-1\"\n", "flat-1"},
		"no slug":     {"runtime = \"node\"\n", ""},
		"bad toml":    {"slug = \n", ""},
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".hatch.toml"), []byte(tc.content), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := AppSlugFromHatchToml(dir); got != tc.want {
			t.Errorf("%s: got %q, want %q", name, got, tc.want)
		}
	}
	if got := AppSlugFromHatchToml(t.TempDir()); got != "" {
		t.Errorf("absent file: got %q", got)
	}
}

// h-8tvak.5: the python build installs from the manifest the project has. A
// pyproject.toml-only project must not be sent to `pip install -r
// requirements.txt`; the generated build steps are run against a fake pip that
// fails like pip does on a missing file, in a project holding only that manifest.
func TestWorkflow_PythonInstallFollowsManifest(t *testing.T) {
	cases := []struct {
		name         string
		manifest     string            // WorkflowParams.PythonManifest
		files        map[string]string // what the checked-out project contains
		wantInstalls []string
		wantNoText   string // must not appear anywhere in the workflow
	}{
		{"requirements.txt only", PythonRequirements, map[string]string{"requirements.txt": "flask\n"}, []string{"-r requirements.txt"}, "pip install ."},
		{"pyproject.toml only", PythonPyproject, map[string]string{"pyproject.toml": "[project]\nname = \"x\"\n"}, []string{"."}, "requirements.txt"},
		{"both: requirements.txt wins", PythonRequirements, map[string]string{"requirements.txt": "flask\n", "pyproject.toml": "[project]\nname = \"x\"\n"}, []string{"-r requirements.txt"}, "pip install ."},
	}
	for _, provider := range []string{"github", "gitlab"} {
		for _, tc := range cases {
			t.Run(provider+"/"+tc.name, func(t *testing.T) {
				p := validParams("python")
				p.PythonManifest = tc.manifest
				wf, err := RenderWorkflow(provider, p)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(wf.Content, tc.wantNoText) {
					t.Errorf("workflow must not contain %q:\n%s", tc.wantNoText, wf.Content)
				}
				cmds, err := workflowtest.BuildCommands(provider, wf.Content)
				if err != nil {
					t.Fatalf("%v\n%s", err, wf.Content)
				}
				project := t.TempDir()
				for name, content := range tc.files {
					if err := os.WriteFile(filepath.Join(project, name), []byte(content), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				installs, out, err := workflowtest.RunBuild(cmds, project)
				if err != nil {
					t.Fatalf("build step failed in a project holding %v: %v\n%s\n%s", tc.files, err, out, wf.Content)
				}
				if !reflect.DeepEqual(installs, tc.wantInstalls) {
					t.Errorf("pip install requests = %q, want %q", installs, tc.wantInstalls)
				}
			})
		}
	}
}

// h-8tvak.5: guard the harness — the old requirements.txt-only workflow really
// does fail in a pyproject.toml-only project, so the test above proves something.
func TestWorkflow_PythonRequirementsStepFailsWithoutRequirementsFile(t *testing.T) {
	wf, err := RenderWorkflow("github", validParams("python")) // requirements.txt install
	if err != nil {
		t.Fatal(err)
	}
	cmds, err := workflowtest.BuildCommands("github", wf.Content)
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "pyproject.toml"), []byte("[project]\nname = \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, out, err := workflowtest.RunBuild(cmds, project); err == nil || !strings.Contains(out, "Could not open requirements file") {
		t.Fatalf("expected the requirements.txt install to fail in a pyproject-only project; err=%v out=%q", err, out)
	}
}

// h-8tvak.5: a python workflow with no usable manifest is refused before it is
// rendered, for both providers; other runtimes ignore the field.
func TestRenderWorkflow_PythonNeedsAManifest(t *testing.T) {
	for _, provider := range []string{"github", "gitlab"} {
		for _, manifest := range []string{"", "Pipfile", "requirements.txt; printf injected"} {
			p := validParams("python")
			p.PythonManifest = manifest
			_, err := RenderWorkflow(provider, p)
			if err == nil || !strings.Contains(err.Error(), "no Python dependency manifest") {
				t.Errorf("%s manifest %q: err = %v, want a refusal naming the missing manifest", provider, manifest, err)
			}
		}
		p := validParams("node")
		p.PythonManifest = "" // not python: the manifest is irrelevant
		if _, err := RenderWorkflow(provider, p); err != nil {
			t.Errorf("%s node: %v", provider, err)
		}
	}
}
