package deploy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
)

// mockAPIClient gains preview support (h-qtie8): recorded so tests can
// assert the deploy path created/refreshed the right preview.
func (m *mockAPIClient) CreatePreview(parentSlug string, prNumber int) (*api.Preview, error) {
	if m.createPreviewFn != nil {
		return m.createPreviewFn(parentSlug, prNumber)
	}
	return &api.Preview{
		Slug:     parentSlug + "-pr-" + itoa(prNumber),
		PRNumber: prNumber,
		URL:      "https://" + parentSlug + "-pr-" + itoa(prNumber) + ".nest.gethatch.eu",
	}, nil
}

func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// writeParentHatchToml drops a .hatch.toml naming the parent egg and chdirs
// into the directory (resolveApp reads cwd), restoring on cleanup.
func writeParentHatchToml(t *testing.T, slug string) string {
	t.Helper()
	tmp := t.TempDir()
	toml := "[app]\nslug = \"" + slug + "\"\nname = \"" + slug + "\"\n"
	if err := os.WriteFile(filepath.Join(tmp, ".hatch.toml"), []byte(toml), 0644); err != nil {
		t.Fatal(err)
	}
	oldDir, _ := os.Getwd()
	os.Chdir(tmp)
	t.Cleanup(func() { os.Chdir(oldDir) })
	return tmp
}

func resetDeployState() {
	deps = defaultDeps()
	deployTarget = ""
	runtime = ""
	startCommand = ""
	previewRef = ""
	jsonOut = false
}

// --- T-010: hatch deploy --preview pr-42 --------------------------------

// The deploy command must register the --preview and --json flags.
func TestNewCmd_PreviewAndJSONFlagsRegistered(t *testing.T) {
	cmd := NewCmd()
	if cmd.Flags().Lookup("preview") == nil {
		t.Error("deploy command missing --preview flag")
	}
	if cmd.Flags().Lookup("json") == nil {
		t.Error("deploy command missing --json flag")
	}
}

// --preview pr-42 on a parent egg: the API preview is created for
// (parent slug from .hatch.toml, 42), the artifact is uploaded to the
// PREVIEW slug (not the parent), and the preview URL is printed.
func TestRunDeploy_Preview_CreatesAndUploadsToPreviewSlug(t *testing.T) {
	tmp := writeParentHatchToml(t, "mysite-x1y2")

	var gotParent string
	var gotPR int
	var uploadedSlug string
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		GetCwd:   func() (string, error) { return tmp, nil },
		NewAPIClient: newMockAPIClient(&mockAPIClient{
			createPreviewFn: func(parentSlug string, prNumber int) (*api.Preview, error) {
				gotParent = parentSlug
				gotPR = prNumber
				return &api.Preview{
					Slug:     "mysite-x1y2-pr-42",
					PRNumber: 42,
					URL:      "https://mysite-x1y2-pr-42.nest.gethatch.eu",
				}, nil
			},
			uploadArtifactFn: func(slug string, artifact []byte, rt, sc string) error {
				uploadedSlug = slug
				return nil
			},
		}),
	}
	defer resetDeployState()

	deployTarget = tmp
	runtime = "static"
	previewRef = "pr-42"

	out := captureOutput(func() {
		if err := runDeploy(nil, nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	if gotParent != "mysite-x1y2" || gotPR != 42 {
		t.Fatalf("CreatePreview called with (%q, %d), want (\"mysite-x1y2\", 42)", gotParent, gotPR)
	}
	if uploadedSlug != "mysite-x1y2-pr-42" {
		t.Fatalf("artifact uploaded to %q, want the preview slug \"mysite-x1y2-pr-42\"", uploadedSlug)
	}
	if !strings.Contains(out, "https://mysite-x1y2-pr-42.nest.gethatch.eu") {
		t.Fatalf("output must print the preview URL, got:\n%s", out)
	}
}

// A bare PR number is accepted as the preview ref.
func TestRunDeploy_Preview_AcceptsBareNumber(t *testing.T) {
	tmp := writeParentHatchToml(t, "mysite-x1y2")

	var gotPR int
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		GetCwd:   func() (string, error) { return tmp, nil },
		NewAPIClient: newMockAPIClient(&mockAPIClient{
			createPreviewFn: func(parentSlug string, prNumber int) (*api.Preview, error) {
				gotPR = prNumber
				return &api.Preview{Slug: "mysite-x1y2-pr-7", PRNumber: prNumber, URL: "https://mysite-x1y2-pr-7.nest.gethatch.eu"}, nil
			},
		}),
	}
	defer resetDeployState()

	deployTarget = tmp
	runtime = "static"
	previewRef = "7"

	captureOutput(func() {
		if err := runDeploy(nil, nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	if gotPR != 7 {
		t.Fatalf("CreatePreview PR = %d, want 7", gotPR)
	}
}

// Garbage refs and non-positive PR numbers are rejected before any API
// call (pr_number 0/negative is invalid per spec).
func TestRunDeploy_Preview_InvalidRefRejected(t *testing.T) {
	for _, ref := range []string{"abc", "pr-abc", "pr-0", "0", "-3", "pr--1"} {
		t.Run(ref, func(t *testing.T) {
			tmp := writeParentHatchToml(t, "mysite-x1y2")

			created := false
			deps = &Deps{
				GetToken: func() (string, error) { return "tok123", nil },
				GetCwd:   func() (string, error) { return tmp, nil },
				NewAPIClient: newMockAPIClient(&mockAPIClient{
					createPreviewFn: func(string, int) (*api.Preview, error) {
						created = true
						return nil, nil
					},
				}),
			}
			defer resetDeployState()

			deployTarget = tmp
			runtime = "static"
			previewRef = ref

			var err error
			captureOutput(func() { err = runDeploy(nil, nil) })

			if err == nil {
				t.Fatalf("preview ref %q: expected an error, got nil", ref)
			}
			if !strings.Contains(err.Error(), "preview") {
				t.Fatalf("error should mention the preview ref, got: %v", err)
			}
			if created {
				t.Fatal("invalid ref must not reach the API")
			}
		})
	}
}

// --json prints one machine-readable JSON object holding the preview
// deploy result (slug, url, pr_number) for the GitHub Action to consume.
func TestRunDeploy_Preview_JSONOutput(t *testing.T) {
	tmp := writeParentHatchToml(t, "mysite-x1y2")

	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		GetCwd:   func() (string, error) { return tmp, nil },
		NewAPIClient: newMockAPIClient(&mockAPIClient{
			createPreviewFn: func(parentSlug string, prNumber int) (*api.Preview, error) {
				return &api.Preview{
					Slug:     "mysite-x1y2-pr-42",
					PRNumber: 42,
					URL:      "https://mysite-x1y2-pr-42.nest.gethatch.eu",
				}, nil
			},
		}),
	}
	defer resetDeployState()

	deployTarget = tmp
	runtime = "static"
	previewRef = "pr-42"
	jsonOut = true

	out := captureOutput(func() {
		if err := runDeploy(nil, nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	var payload struct {
		Slug     string `json:"slug"`
		URL      string `json:"url"`
		PRNumber int    `json:"pr_number"`
	}
	jsonLine := ""
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "{") {
			jsonLine = line
			break
		}
	}
	if jsonLine == "" {
		t.Fatalf("--json output contains no JSON object, got:\n%s", out)
	}
	if err := json.Unmarshal([]byte(jsonLine), &payload); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\n%s", err, jsonLine)
	}
	if payload.Slug != "mysite-x1y2-pr-42" || payload.PRNumber != 42 || !strings.Contains(payload.URL, "mysite-x1y2-pr-42") {
		t.Fatalf("JSON payload = %+v, want slug/url/pr_number of the preview", payload)
	}
}
