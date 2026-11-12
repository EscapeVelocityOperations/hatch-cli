package ci

import (
	"debug/elf"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/ci/workflowtest"
)

// writeTree writes files (slash paths → content) under dir.
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

const (
	mainSrc    = "package main\n\nfunc main() {}\n"
	libSrc     = "package lib\n\nfunc Name() string { return \"x\" }\n"
	exampleSrc = "package example\n\nfunc Name() string { return \"x\" }\n"
)

// multiPackageFixture is the project of h-8tvak.4: cmd/server + internal/example.
// `go build ./...` compiles it and writes no executable.
var multiPackageFixture = map[string]string{
	"go.mod":                      "module example.com/fix\n\ngo 1.22\n",
	"internal/example/example.go": exampleSrc,
	"cmd/server/main.go":          "package main\n\nimport (\n\t\"fmt\"\n\n\t\"example.com/fix/internal/example\"\n)\n\nfunc main() { fmt.Println(example.Name()) }\n",
}

func TestGoBinaryPath(t *testing.T) {
	good := map[string]string{
		"./server":               "server",
		"./bin/server --port 80": "bin/server",
		"  ./server  ":           "server",
		"./a/../server":          "server",
		"./my-app_v1.2":          "my-app_v1.2",
	}
	for in, want := range good {
		if got, err := GoBinaryPath(in); err != nil || got != want {
			t.Errorf("GoBinaryPath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{
		"", "   ", "/usr/bin/app", "../server", "./..", ".", "./", "./../server", "./a/../../server",
		"server", "bin/server", // a binary is named with an explicit ./
		"sh -c ./server", "env FOO=1 ./server", "exec ./server", // wrappers: their own name would be built
		`"./my server"`, "$HOME/server", "$(printf x)", "./$X", "./server;", "./a|b", "~/server", "./server\\x",
	} {
		if got, err := GoBinaryPath(in); err == nil {
			t.Errorf("GoBinaryPath(%q) = %q, want a refusal", in, got)
		}
	}
}

func TestValidateGoPackage(t *testing.T) {
	for _, ok := range []string{".", "./cmd/server", "./a", "./cmd/my-app", "./x/y.z", "./cmd/v2+"} {
		if err := ValidateGoPackage(ok); err != nil {
			t.Errorf("ValidateGoPackage(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{
		"", "cmd/server", "-ldflags=-X", "./...", "./cmd/...", "../x", "./../x", "./cmd/..", "./cmd/./x",
		"./cmd/server/", "/abs/path", "./a b", "./$(x)", "./a;b", "example.com/fix/cmd/server", "..", "./",
	} {
		if err := ValidateGoPackage(bad); err == nil {
			t.Errorf("ValidateGoPackage(%q) accepted", bad)
		}
	}
}

func TestFindGoMainPackages(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"go.mod":                      "module example.com/fix\n\ngo 1.22\n",
		"main.go":                     mainSrc,
		"cmd/server/main.go":          mainSrc,
		"cmd/server/main_test.go":     mainSrc, // tests do not count
		"cmd/worker/main.go":          mainSrc,
		"internal/lib/lib.go":         libSrc,
		"cmd/tool/gen.go":             "//go:build ignore\n\n" + mainSrc, // excluded by a build constraint
		"cmd/winonly/main_windows.go": mainSrc,                           // not linux
		"cmd/mixed/a.go":              mainSrc,
		"cmd/mixed/b.go":              libSrc, // two package names: unbuildable
		"vendor/x/main.go":            mainSrc,
		"node_modules/y/main.go":      mainSrc,
		"testdata/z/main.go":          mainSrc,
		".hidden/main.go":             mainSrc,
		"_skip/main.go":               mainSrc,
		"nested/go.mod":               "module example.com/nested\n\ngo 1.22\n",
		"nested/main.go":              mainSrc, // another module
		"docs/readme.md":              "# x\n",
	})
	got, err := FindGoMainPackages(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".", "./cmd/server", "./cmd/worker"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FindGoMainPackages = %q, want %q", got, want)
	}

	got, err = FindGoMainPackages(t.TempDir())
	if err != nil || len(got) != 0 {
		t.Errorf("empty dir: %q, %v", got, err)
	}
	if _, err := FindGoMainPackages(filepath.Join(dir, "absent")); err == nil {
		t.Error("absent dir: expected an error")
	}
}

func TestChooseGoMainPackage(t *testing.T) {
	cases := []struct {
		name       string
		candidates []string
		start      string
		want       string
		wantErr    string
	}{
		{"none", nil, "./server", "", "no Go main package found"},
		{"only one", []string{"./cmd/api"}, "./server", "./cmd/api", ""},
		{"only one, start command unusable", []string{"./cmd/api"}, "", "./cmd/api", ""},
		{"named like the binary", []string{"./cmd/server", "./cmd/worker"}, "./server", "./cmd/server", ""},
		{"named, binary in a subdirectory", []string{"./cmd/server", "./cmd/worker"}, "./bin/worker --x", "./cmd/worker", ""},
		{"start command without ./ is not used to pick", []string{"./cmd/server", "./cmd/worker"}, "server", "", "several Go main packages"},
		{"none named like the binary", []string{"./cmd/a", "./cmd/b"}, "./server", "", "several Go main packages"},
		{"two named like the binary", []string{"./cmd/server", "./tools/server"}, "./server", "", "several Go main packages"},
		{"root is never matched by name", []string{".", "./cmd/a"}, "./server", "", "several Go main packages"},
		{"unusable start command", []string{"./cmd/a", "./cmd/b"}, "", "", "several Go main packages"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ChooseGoMainPackage(tc.candidates, tc.start)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				if tc.wantErr == "several Go main packages" && !strings.Contains(err.Error(), "--go-package") {
					t.Errorf("error does not tell how to choose: %v", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestGoBuildOutput(t *testing.T) {
	for _, tc := range []struct{ target, start, want string }{
		{"dist", "./server", "dist/server"},
		{"./dist/", "./server --port 80", "dist/server"},
		{"build out", "./bin/app", "build out/bin/app"},
		{".", "./server", "server"},
	} {
		got, err := GoBuildOutput(WorkflowParams{DeployTarget: tc.target, StartCommand: tc.start})
		if err != nil || got != tc.want {
			t.Errorf("GoBuildOutput(%q, %q) = %q, %v; want %q", tc.target, tc.start, got, err, tc.want)
		}
	}
	if _, err := GoBuildOutput(WorkflowParams{DeployTarget: "dist", StartCommand: "$HOME/x"}); err == nil {
		t.Error("expected a refusal for a start command that does not name a binary")
	}
}

// h-8tvak.4: the Go workflow must produce, in the deploy-target, the binary the
// start command runs. `go build ./...` on this multi-package checkout exits 0
// and writes nothing; the generated build step is run for real on it and the
// artifact `hatch deploy` would receive is inspected. Covers both providers,
// with a deploy-target that has a space and a binary in a subdirectory.
func TestGoWorkflow_BuildStepProducesTheDeployedBinary(t *testing.T) {
	for _, provider := range []string{"github", "gitlab"} {
		for _, tc := range []struct{ name, target, start, wantFile string }{
			{"simple", "dist", "./server", "dist/server"},
			{"target with a space, nested binary", "build out", "./bin/server --port 8080", "build out/bin/server"},
			{"target is the root", ".", "./server", "server"},
		} {
			t.Run(provider+"/"+tc.name, func(t *testing.T) {
				checkout := t.TempDir()
				writeTree(t, checkout, multiPackageFixture)

				p := validParams("go")
				p.DeployTarget, p.StartCommand, p.GoPackage = tc.target, tc.start, "./cmd/server"
				wf, err := RenderWorkflow(provider, p)
				if err != nil {
					t.Fatal(err)
				}
				d, err := workflowtest.Extract(provider, wf.Content)
				if err != nil {
					t.Fatalf("%v\n%s", err, wf.Content)
				}
				if len(d.Build) == 0 {
					t.Fatalf("no build step before the deploy step:\n%s", wf.Content)
				}
				if strings.Contains(strings.Join(scripts(d), "\n"), "go build ./...") {
					t.Errorf("workflow still builds with `go build ./...`:\n%s", wf.Content)
				}

				out, err := workflowtest.RunBuild(d, checkout, nil)
				if err != nil {
					t.Fatalf("build failed: %v\n%s", err, out)
				}
				bin := filepath.Join(checkout, filepath.FromSlash(tc.wantFile))
				f, err := elf.Open(bin)
				if err != nil {
					t.Fatalf("the build did not leave a linux binary at %s: %v", tc.wantFile, err)
				}
				defer f.Close()
				if f.Machine != elf.EM_X86_64 || f.Class != elf.ELFCLASS64 {
					t.Errorf("binary is %v/%v, want linux amd64", f.Class, f.Machine)
				}
				if f.Section(".interp") != nil {
					t.Error("binary is dynamically linked: want CGO_ENABLED=0")
				}
				if fi, _ := os.Stat(bin); fi.Mode()&0o111 == 0 {
					t.Errorf("binary %s is not executable", tc.wantFile)
				}

				// The deploy step then receives that directory and start command.
				res, err := workflowtest.Run(d, checkout, workflowtest.FakeHatch, nil)
				if err != nil || res.Err != nil {
					t.Fatalf("%v %v\n%s", err, res.Err, res.Output)
				}
				want := []string{"deploy", "--app", "fixture-stage", "--runtime", "go",
					"--deploy-target", tc.target, "--start-command", tc.start}
				if !reflect.DeepEqual(res.Argv, want) {
					t.Errorf("argv = %q, want %q", res.Argv, want)
				}
			})
		}
	}
}

func scripts(d workflowtest.Deploy) []string {
	var out []string
	for _, s := range d.Build {
		out = append(out, s.Script)
	}
	return out
}

// The build step reads its values as data: a hostile deploy-target or start
// command never reaches the shell as code, and the build still lands where the
// deploy step looks.
func TestGoWorkflow_BuildStepTreatsParametersAsData(t *testing.T) {
	for _, provider := range []string{"github", "gitlab"} {
		t.Run(provider, func(t *testing.T) {
			checkout := t.TempDir()
			writeTree(t, checkout, multiPackageFixture)
			p := validParams("go")
			p.DeployTarget = "o'ut $(printf vigie-executed) `printf vigie-backtick`; printf injected"
			p.StartCommand = "./server"
			p.GoPackage = "./cmd/server"
			wf, err := RenderWorkflow(provider, p)
			if err != nil {
				t.Fatal(err)
			}
			d, err := workflowtest.Extract(provider, wf.Content)
			if err != nil {
				t.Fatalf("%v\n%s", err, wf.Content)
			}
			for _, s := range scripts(d) {
				if strings.Contains(s, p.DeployTarget) || strings.Contains(s, "cmd/server") {
					t.Errorf("a parameter is spliced into the build script:\n%s", s)
				}
			}
			if provider == "gitlab" {
				for k := range d.Env {
					if !d.NoExpand[k] {
						t.Errorf("gitlab variable %s must be expand: false", k)
					}
				}
			}
			out, err := workflowtest.RunBuild(d, checkout, nil)
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			if strings.Contains(out, "vigie-") || strings.Contains(out, "injected") {
				t.Errorf("a parameter was executed by the shell:\n%s", out)
			}
			if _, err := os.Stat(filepath.Join(checkout, p.DeployTarget, "server")); err != nil {
				t.Errorf("binary not built into the deploy-target: %v", err)
			}
		})
	}
}

func TestVerifyGoMainPackage(t *testing.T) {
	cands := []string{"./cmd/server", "./cmd/worker"}
	if err := VerifyGoMainPackage(cands, "./cmd/worker"); err != nil {
		t.Errorf("a found main package was refused: %v", err)
	}
	err := VerifyGoMainPackage(cands, "./internal/example")
	if err == nil || !strings.Contains(err.Error(), "not a main package") || !strings.Contains(err.Error(), "./cmd/server") {
		t.Errorf("a library package must be refused, listing the main packages: %v", err)
	}
	if err := VerifyGoMainPackage(nil, "./cmd/server"); err == nil {
		t.Error("no main package at all: expected a refusal")
	}
}

// h-8tvak.4: a go workflow that cannot name the binary and the package it
// builds is refused, never rendered as one that compiles and deploys nothing.
func TestRenderWorkflow_GoRefusesWhatItCannotBuild(t *testing.T) {
	cases := map[string]func(p *WorkflowParams){
		"no main package chosen":     func(p *WorkflowParams) { p.GoPackage = "" },
		"start command is a wrapper": func(p *WorkflowParams) { p.StartCommand = "sh -c ./server" },
		"start command is a bare name": func(p *WorkflowParams) {
			p.StartCommand = "server"
		},
		"start command escapes the target": func(p *WorkflowParams) { p.StartCommand = "./../server" },
		"package is a flag":                func(p *WorkflowParams) { p.GoPackage = "-ldflags=-X" },
		"package is a wildcard":            func(p *WorkflowParams) { p.GoPackage = "./..." },
		"package climbs out":               func(p *WorkflowParams) { p.GoPackage = "../other" },
		"package is an import path":        func(p *WorkflowParams) { p.GoPackage = "example.com/fix/cmd/server" },
	}
	for name, mut := range cases {
		for _, provider := range []string{"github", "gitlab"} {
			t.Run(provider+"/"+name, func(t *testing.T) {
				p := validParams("go")
				mut(&p)
				if wf, err := RenderWorkflow(provider, p); err == nil {
					t.Fatalf("expected refusal, got workflow:\n%s", wf.Content)
				}
			})
		}
	}
	// --go-package means nothing to another runtime.
	for _, rt := range []string{"node", "python", "rust", "php", "bun", "static"} {
		p := validParams(rt)
		p.GoPackage = "./cmd/server"
		if err := p.ValidateInputs(); err == nil || !strings.Contains(err.Error(), "go runtime") {
			t.Errorf("%s with a go package: err = %v", rt, err)
		}
	}
	// Other runtimes render without any go build variable.
	for _, provider := range []string{"github", "gitlab"} {
		wf, err := RenderWorkflow(provider, validParams("node"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(wf.Content, "HATCH_CI_GO_") || strings.Contains(wf.Content, "GOOS") {
			t.Errorf("%s/node workflow carries go build settings:\n%s", provider, wf.Content)
		}
	}
}
