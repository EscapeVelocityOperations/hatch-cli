// Package scripts also guards .github/workflows/release.yml (T-001, h-6ewk):
// darwin legs must run on a macOS runner and sign+notarize before upload,
// gated so absent Apple secrets fall back to today's unsigned behavior.
package scripts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type releaseWorkflow struct {
	Jobs map[string]releaseWorkflowJob `yaml:"jobs"`
}

type releaseWorkflowJob struct {
	RunsOn   string                `yaml:"runs-on"`
	Env      map[string]string     `yaml:"env"`
	Strategy releaseWorkflowStrat  `yaml:"strategy"`
	Steps    []releaseWorkflowStep `yaml:"steps"`
}

type releaseWorkflowStrat struct {
	Matrix releaseWorkflowMatrix `yaml:"matrix"`
}

type releaseWorkflowMatrix struct {
	Include []map[string]string `yaml:"include"`
}

type releaseWorkflowStep struct {
	Name string `yaml:"name"`
	If   string `yaml:"if"`
	Run  string `yaml:"run"`
}

func readReleaseWorkflow(t *testing.T) releaseWorkflow {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("reading release.yml: %v", err)
	}
	var wf releaseWorkflow
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("parsing release.yml: %v", err)
	}
	return wf
}

func releaseBuildJob(t *testing.T, wf releaseWorkflow) releaseWorkflowJob {
	t.Helper()
	job, ok := wf.Jobs["build"]
	if !ok {
		t.Fatalf("release.yml has no %q job", "build")
	}
	return job
}

func darwinLegs(job releaseWorkflowJob) []map[string]string {
	var legs []map[string]string
	for _, leg := range job.Strategy.Matrix.Include {
		if leg["goos"] == "darwin" {
			legs = append(legs, leg)
		}
	}
	return legs
}

// stepContaining returns the first step whose run script contains every
// substring in substrs, or nil if none match.
func stepContaining(steps []releaseWorkflowStep, substrs ...string) *releaseWorkflowStep {
	for i := range steps {
		s := &steps[i]
		all := true
		for _, sub := range substrs {
			if !strings.Contains(s.Run, sub) {
				all = false
				break
			}
		}
		if all {
			return s
		}
	}
	return nil
}

// TestReleaseWorkflow guards the darwin release legs (T-001, h-6ewk): a
// macOS runner, sign+notarize steps gated on the Apple secrets, and an
// unsigned-warning fallback when those secrets are absent. Each subtest
// is a lettered acceptance criterion from the h-6ewk plan.
func TestReleaseWorkflow(t *testing.T) {
	wf := readReleaseWorkflow(t)
	job := releaseBuildJob(t, wf)

	t.Run("a_darwin_legs_use_macos_runner", func(t *testing.T) {
		legs := darwinLegs(job)
		if len(legs) == 0 {
			t.Fatalf("no darwin legs found in build job matrix")
		}
		for _, leg := range legs {
			if !strings.Contains(leg["runner"], "macos") {
				t.Errorf("darwin leg %v: runner = %q, want a macos-* runner", leg, leg["runner"])
			}
		}
		if job.RunsOn != "${{ matrix.runner }}" {
			t.Errorf("build job runs-on = %q, want %q", job.RunsOn, "${{ matrix.runner }}")
		}
	})

	t.Run("b_codesign_step_exists", func(t *testing.T) {
		step := stepContaining(job.Steps, "codesign", "--options runtime", "--timestamp")
		if step == nil {
			t.Fatalf("no step found with codesign --options runtime --timestamp")
		}
	})

	t.Run("c_notarize_step_exists", func(t *testing.T) {
		step := stepContaining(job.Steps, "notarytool submit", "--wait")
		if step == nil {
			t.Fatalf("no step found with notarytool submit --wait")
		}
	})

	t.Run("d_sign_and_notarize_steps_are_env_gated", func(t *testing.T) {
		sign := stepContaining(job.Steps, "codesign", "--options runtime")
		if sign == nil {
			t.Fatalf("no codesign step found to check gating")
		}
		if !strings.Contains(sign.If, "env.APPLE_SIGNING_AVAILABLE == 'true'") {
			t.Errorf("codesign step if = %q, want it to reference env.APPLE_SIGNING_AVAILABLE == 'true'", sign.If)
		}

		notarize := stepContaining(job.Steps, "notarytool submit")
		if notarize == nil {
			t.Fatalf("no notarize step found to check gating")
		}
		if !strings.Contains(notarize.If, "env.APPLE_SIGNING_AVAILABLE == 'true'") {
			t.Errorf("notarize step if = %q, want it to reference env.APPLE_SIGNING_AVAILABLE == 'true'", notarize.If)
		}
	})

	t.Run("e_unsigned_warning_step_exists", func(t *testing.T) {
		step := stepContaining(job.Steps, "::warning::", "UNSIGNED")
		if step == nil {
			t.Fatalf("no unsigned-warning step found (expected ::warning:: ... UNSIGNED ...)")
		}
		if !strings.Contains(step.If, "env.APPLE_SIGNING_AVAILABLE != 'true'") {
			t.Errorf("unsigned-warning step if = %q, want it to reference env.APPLE_SIGNING_AVAILABLE != 'true'", step.If)
		}
	})

	t.Run("f_goreleaser_config_removed", func(t *testing.T) {
		_, err := os.Stat(filepath.Join("..", ".goreleaser.yml"))
		if !os.IsNotExist(err) {
			t.Errorf(".goreleaser.yml still exists (want removed): err=%v", err)
		}
	})

	// g+h guard the h-6ewk infra-review B1 finding: the `secrets` context is
	// illegal in step-level `if` (GitHub Actions context-availability rules;
	// actionlint enforces it). Signing/notarize gating must go through a
	// job-level env indirection instead.
	t.Run("g_job_env_wires_signing_available_from_secrets", func(t *testing.T) {
		val, ok := job.Env["APPLE_SIGNING_AVAILABLE"]
		if !ok {
			t.Fatalf("build job has no env.APPLE_SIGNING_AVAILABLE")
		}
		if !strings.Contains(val, "secrets.APPLE_CERT_P12") {
			t.Errorf("job env APPLE_SIGNING_AVAILABLE = %q, want it derived from secrets.APPLE_CERT_P12", val)
		}
	})

	t.Run("h_no_step_if_references_secrets_directly", func(t *testing.T) {
		for _, step := range job.Steps {
			if strings.Contains(step.If, "secrets.") {
				t.Errorf("step %q if = %q references secrets directly (illegal in step-level if) — gate on env.APPLE_SIGNING_AVAILABLE instead", step.Name, step.If)
			}
		}
	})
}
