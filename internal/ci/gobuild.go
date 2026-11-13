package ci

import (
	"errors"
	"fmt"
	"go/build"
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// A Go project is deployed as one executable. `go build ./...` only checks that
// every package compiles: with several packages it writes no binary, so a
// workflow ending there would hand `hatch deploy` an empty or missing
// deploy-target (h-8tvak.4). The Go workflow instead builds ONE main package,
// as linux/amd64 (what Hatch runs), to the path its start command executes
// inside the deploy-target. Both values are resolved when `hatch ci` runs and
// written into the workflow, where the user reviews them.

var (
	// goBinaryWordRe is the shape of the start command's first word when it names
	// the binary: an explicit "./" relative path, nothing a shell would interpret.
	// Requiring "./" tells a binary ("./server") from a wrapper ("sh -c …",
	// "env X=1 …") whose name would otherwise be built by mistake.
	goBinaryWordRe = regexp.MustCompile(`^\./[A-Za-z0-9._+/-]+$`)
	goPackageSegRe = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)
)

// GoBinaryPath returns the path, relative to the deploy-target, of the binary a
// go start command runs: its first word, which must be an explicit relative
// path, e.g. "./server" or "./bin/server --port 80" → "server" / "bin/server".
// A start command that does not begin that way (a shell wrapper, an env
// assignment, a bare name, a quoted or absolute path, a non-canonical path such
// as "./a/../server" or "./server/") is refused: the binary to build cannot be
// told from it, or would not be where the start command looks.
func GoBinaryPath(startCommand string) (string, error) {
	fields := strings.Fields(startCommand)
	if len(fields) == 0 {
		return "", errors.New("--start-command is required for runtime go")
	}
	rel := path.Clean(fields[0])
	if !goBinaryWordRe.MatchString(fields[0]) || rel == "." || rel == ".." ||
		strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, "../") ||
		// h-7olu2.1: the workflow builds to the cleaned path but the deploy runs
		// the start command as written, so the two must be the same path: a
		// "..", "." or empty segment, or a trailing slash, is refused.
		fields[0] != "./"+rel {
		return "", fmt.Errorf("cannot tell which binary --start-command %q runs: for runtime go its first word "+
			"must be ./ and the path of the binary inside --deploy-target (for example ./server)", startCommand)
	}
	return rel, nil
}

// ValidateGoPackage checks a main-package path: "." (the module root) or a
// "./dir/sub" path inside the module. It cannot start with "-" (a flag to go
// build), climb out with "..", or be a "..." wildcard.
func ValidateGoPackage(pkg string) error {
	if pkg == "." {
		return nil
	}
	rest, ok := strings.CutPrefix(pkg, "./")
	valid := ok
	if ok {
		for _, seg := range strings.Split(rest, "/") {
			if !goPackageSegRe.MatchString(seg) || strings.Trim(seg, ".") == "" {
				valid = false
			}
		}
	}
	if !valid {
		return fmt.Errorf("invalid --go-package %q: use a directory inside the module such as ./cmd/server (or . for the module root)", pkg)
	}
	return nil
}

// GoBuildOutput is the path `go build -o` writes the binary to: the start
// command's binary inside the deploy-target.
func GoBuildOutput(p WorkflowParams) (string, error) {
	bin, err := GoBinaryPath(p.StartCommand)
	if err != nil {
		return "", err
	}
	return path.Join(p.DeployTarget, bin), nil
}

// FindGoMainPackages lists the main packages of the Go module rooted at dir, as
// "." or "./sub/dir" paths in lexical order. It reads source files as
// linux/amd64 without cgo — what the workflow builds — and skips vendor,
// node_modules, testdata, hidden and underscore directories and nested modules.
func FindGoMainPackages(dir string) ([]string, error) {
	ctx := build.Default
	ctx.GOOS, ctx.GOARCH, ctx.CgoEnabled = "linux", "amd64", false
	var found []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == dir {
				return err
			}
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if p != dir {
			name := d.Name()
			if name == "vendor" || name == "node_modules" || name == "testdata" ||
				strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return fs.SkipDir
			}
			if fileExists(filepath.Join(p, "go.mod")) { // a nested module is not built from the root
				return fs.SkipDir
			}
		}
		pkg, err := ctx.ImportDir(p, 0)
		if err != nil || pkg.Name != "main" {
			return nil // no Go files, mixed package names, or not a command
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return nil
		}
		if rel == "." {
			found = append(found, ".")
		} else {
			found = append(found, "./"+filepath.ToSlash(rel))
		}
		return nil
	})
	return found, err
}

// VerifyGoMainPackage checks that an explicitly chosen package is one of the
// main packages found in the checkout: building a library with `go build -o`
// would write a non-executable and deploy it.
func VerifyGoMainPackage(candidates []string, pkg string) error {
	for _, c := range candidates {
		if c == pkg {
			return nil
		}
	}
	if len(candidates) == 0 {
		return fmt.Errorf("--go-package %s is not a main package: no package main found in this directory", pkg)
	}
	return fmt.Errorf("--go-package %s is not a main package (found: %s)", pkg, strings.Join(candidates, ", "))
}

// ChooseGoMainPackage picks the package to build out of candidates, or refuses
// when the choice is not unambiguous: the only main package; else the only one
// whose directory is named like the binary the start command runs.
func ChooseGoMainPackage(candidates []string, startCommand string) (string, error) {
	switch len(candidates) {
	case 0:
		return "", errors.New("no Go main package found: pass --go-package with the directory of the package main to build (for example ./cmd/server)")
	case 1:
		return candidates[0], nil
	}
	if bin, err := GoBinaryPath(startCommand); err == nil {
		var named []string
		for _, c := range candidates {
			if c != "." && path.Base(c) == path.Base(bin) {
				named = append(named, c)
			}
		}
		if len(named) == 1 {
			return named[0], nil
		}
	}
	return "", fmt.Errorf("several Go main packages found (%s): pass --go-package with the one to deploy", strings.Join(candidates, ", "))
}
