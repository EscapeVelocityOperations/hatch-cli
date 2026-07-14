package ci

import (
	"strings"
	"testing"
)

// h-tymh/h-e2h5: RenderWorkflow produces a correct deploy workflow per
// provider × runtime. Rather than byte-exact golden files (brittle across
// cosmetic edits), each case asserts the correctness-critical invariants of the
// generated file: the right path, the push→main trigger, the runtime toolchain
// setup, and the `hatch deploy` command wired to the provider's HATCH_TOKEN secret.
func TestRenderWorkflow(t *testing.T) {
	params := WorkflowParams{Runtime: "", DeployTarget: "dist", StartCommand: "node server.js"}

	cases := []struct {
		provider string
		runtime  string
		wantPath string
		wantAll  []string // substrings that must appear
	}{
		{"github", "node", ".github/workflows/hatch-deploy.yml", []string{
			"push:", "branches: [main]", "actions/setup-node", "hatch deploy", "--runtime node",
			"--deploy-target dist", `--start-command "node server.js"`, "${{ secrets.HATCH_TOKEN }}",
		}},
		{"github", "go", ".github/workflows/hatch-deploy.yml", []string{"actions/setup-go", "--runtime go", "${{ secrets.HATCH_TOKEN }}"}},
		{"github", "python", ".github/workflows/hatch-deploy.yml", []string{"actions/setup-python", "--runtime python"}},
		{"github", "static", ".github/workflows/hatch-deploy.yml", []string{"--runtime static", "hatch deploy"}},
		{"gitlab", "node", ".gitlab-ci.yml", []string{
			"stages:", "deploy:", "only:", "- main", "hatch deploy", "--runtime node", "$HATCH_TOKEN",
		}},
		{"gitlab", "go", ".gitlab-ci.yml", []string{"go build", "--runtime go", "$HATCH_TOKEN"}},
		{"gitlab", "python", ".gitlab-ci.yml", []string{"pip install", "--runtime python"}},
		{"gitlab", "static", ".gitlab-ci.yml", []string{"--runtime static", "$HATCH_TOKEN"}},
	}

	for _, tc := range cases {
		t.Run(tc.provider+"/"+tc.runtime, func(t *testing.T) {
			p := params
			p.Runtime = tc.runtime
			wf, err := RenderWorkflow(tc.provider, p)
			if err != nil {
				t.Fatalf("RenderWorkflow(%s,%s): %v", tc.provider, tc.runtime, err)
			}
			if wf.Path != tc.wantPath {
				t.Errorf("path = %q, want %q", wf.Path, tc.wantPath)
			}
			for _, want := range tc.wantAll {
				if !strings.Contains(wf.Content, want) {
					t.Errorf("%s/%s workflow missing %q\n---\n%s", tc.provider, tc.runtime, want, wf.Content)
				}
			}
			// The GitHub secret ref must never leak into a GitLab file and vice-versa.
			if tc.provider == "gitlab" && strings.Contains(wf.Content, "secrets.HATCH_TOKEN") {
				t.Errorf("gitlab workflow must use $HATCH_TOKEN, not the GitHub secret ref")
			}
		})
	}
}

func TestRenderWorkflow_UnknownProvider(t *testing.T) {
	if _, err := RenderWorkflow("bitbucket", WorkflowParams{Runtime: "node"}); err == nil {
		t.Fatal("expected an error for an unsupported provider")
	}
}

// Empty deploy-target / start-command must omit those flags entirely (not emit
// empty --deploy-target / --start-command "").
func TestRenderWorkflow_OmitsEmptyFlags(t *testing.T) {
	wf, err := RenderWorkflow("github", WorkflowParams{Runtime: "static"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(wf.Content, "--deploy-target") {
		t.Errorf("empty deploy-target must be omitted; got:\n%s", wf.Content)
	}
	if strings.Contains(wf.Content, "--start-command") {
		t.Errorf("empty start-command must be omitted; got:\n%s", wf.Content)
	}
}
