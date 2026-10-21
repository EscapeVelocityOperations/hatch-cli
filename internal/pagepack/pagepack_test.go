package pagepack

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func entries(t *testing.T, archive []byte) map[string]string {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(zr)
	out := map[string]string{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Typeflag != tar.TypeReg {
			t.Fatalf("%s: non-regular entry %c", h.Name, h.Typeflag)
		}
		b, _ := io.ReadAll(tr)
		out[h.Name] = string(b)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuild_SingleHTMLFileBecomesIndex(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "rapport.html"), "<h1>r</h1>")
	p, err := Build(filepath.Join(dir, "rapport.html"))
	if err != nil {
		t.Fatal(err)
	}
	if got := entries(t, p.Archive); len(got) != 1 || got["index.html"] != "<h1>r</h1>" || p.Files != 1 || p.Bytes != 10 {
		t.Fatalf("entries = %v pack=%+v", got, p)
	}
	write(t, filepath.Join(dir, "notes.txt"), "x")
	if _, err := Build(filepath.Join(dir, "notes.txt")); err == nil || !strings.Contains(err.Error(), ".html") {
		t.Fatalf("non-html single file: %v", err)
	}
}

func TestBuild_FolderKeepsRegularFilesOnly(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "index.html"), "<p>i</p>")
	write(t, filepath.Join(dir, "media", "a.m4a"), "AUDIO")
	write(t, filepath.Join(dir, ".env"), "SECRET=1")
	write(t, filepath.Join(dir, ".git", "config"), "x")
	write(t, filepath.Join(dir, "media", ".DS_Store"), "x")
	if err := os.Symlink("/etc/passwd", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	p, err := Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := entries(t, p.Archive)
	if len(got) != 2 || got["index.html"] != "<p>i</p>" || got["media/a.m4a"] != "AUDIO" {
		t.Fatalf("entries = %v", got)
	}
	sort.Strings(p.Skipped)
	if strings.Join(p.Skipped, ",") != ".env,.git,link,media/.DS_Store" {
		t.Fatalf("skipped = %v", p.Skipped)
	}
}

func TestBuild_FolderNeedsIndex(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "page.html"), "x")
	if _, err := Build(dir); err == nil || !strings.Contains(err.Error(), "index.html") {
		t.Fatalf("folder without index: %v", err)
	}
}

func TestBuild_RefusesUnsafeNames(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "index.html"), "x")
	write(t, filepath.Join(dir, "a‮gnp.html"), "x")
	if _, err := Build(dir); err == nil || !strings.Contains(err.Error(), "formatting characters") {
		t.Fatalf("bidi name: %v", err)
	}
}

func TestFromHTML(t *testing.T) {
	p, err := FromHTML("<h1>x</h1>")
	if err != nil || entries(t, p.Archive)["index.html"] != "<h1>x</h1>" {
		t.Fatalf("inline: %v", err)
	}
	if _, err := FromHTML("  "); err == nil {
		t.Fatal("empty html accepted")
	}
	if _, err := FromHTML(strings.Repeat("a", MaxInlineHTML+1)); err == nil {
		t.Fatal("oversized inline html accepted")
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"Bilan d'été 2026": "bilan-d-ete-2026", "rapport_final": "rapport-final", "  --  ": "", "ÉCHÉANCIER": "echeancier",
	} {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Slugify(strings.Repeat("ab-", 40)); len(got) > 80 || strings.HasSuffix(got, "-") {
		t.Errorf("long slug = %q", got)
	}
}
