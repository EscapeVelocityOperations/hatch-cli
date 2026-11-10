// Package pagepack builds the tar.gz a `hatch pages publish` uploads
// (h-8iz8 D6): one HTML file becomes index.html; a directory must hold an
// index.html. Only regular files go in — hatch-control refuses links and
// devices — and hidden files (.git, .env, .DS_Store…) are left out.
package pagepack

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Limits mirror hatch-api's defaults (D7) so most refusals happen before
// the upload; the API stays the authority.
const (
	MaxFileBytes        = 50 << 20
	MaxFiles            = 2000
	MaxPublicationBytes = 200 << 20
	// MaxInlineHTML bounds the html of an MCP publish_page call.
	MaxInlineHTML = 2 << 20
)

// Pack is a built archive.
type Pack struct {
	Archive []byte
	Files   int
	Bytes   int64    // uncompressed
	Skipped []string // hidden files, symlinks and other non-regular entries left out
}

// Build packs path: an .html/.htm file (published as index.html) or a
// directory containing index.html.
func Build(path string) (*Pack, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".html" && ext != ".htm" {
			return nil, fmt.Errorf("%s: a single file must be an .html page (publish a directory for anything else)", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return fromFiles([]file{{name: "index.html", data: data}}, nil)
	}

	// Lstat: a symlinked index.html would pass a Stat, then be skipped by
	// the walk below (links are not published) and refused by the server.
	st, err := os.Lstat(filepath.Join(path, "index.html"))
	if err != nil {
		return nil, fmt.Errorf("%s has no index.html: a published directory needs one", path)
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%s/index.html is not a regular file (a link?): links are not published, copy the file instead", path)
	}
	var files []file
	var skipped []string
	var total int64
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == path {
			return nil
		}
		rel, err := filepath.Rel(path, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(d.Name(), ".") {
			skipped = append(skipped, rel)
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			skipped = append(skipped, rel)
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > MaxFileBytes {
			return fmt.Errorf("%s is %.1f MB: a page file is at most %d MB", rel, float64(info.Size())/(1<<20), MaxFileBytes>>20)
		}
		total += info.Size()
		if total > MaxPublicationBytes {
			return fmt.Errorf("%s holds more than %d MB of files", path, MaxPublicationBytes>>20)
		}
		if len(files) >= MaxFiles {
			return fmt.Errorf("%s holds more than %d files", path, MaxFiles)
		}
		files = append(files, file{name: rel, path: p})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return fromFiles(files, skipped)
}

// FromHTML packs inline HTML as index.html (MCP publish_page).
func FromHTML(html string) (*Pack, error) {
	if strings.TrimSpace(html) == "" {
		return nil, errors.New("html is empty")
	}
	if len(html) > MaxInlineHTML {
		return nil, fmt.Errorf("html is %d bytes: inline pages are at most %d MB (publish a file instead)", len(html), MaxInlineHTML>>20)
	}
	return fromFiles([]file{{name: "index.html", data: []byte(html)}}, nil)
}

type file struct {
	name string
	path string // read from disk when data is nil
	data []byte
	size int64 // set by addFile
}

// addFile writes one regular-file entry, closing the source right away (a
// directory may hold up to MaxFiles files).
func addFile(tw *tar.Writer, f *file) error {
	name := norm.NFC.String(f.name)
	if bad := badName(name); bad != "" {
		return fmt.Errorf("%q: %s", f.name, bad)
	}
	r := io.Reader(bytes.NewReader(f.data))
	f.size = int64(len(f.data))
	if f.data == nil {
		fh, err := os.Open(f.path)
		if err != nil {
			return err
		}
		defer func() { _ = fh.Close() }()
		st, err := fh.Stat()
		if err != nil {
			return err
		}
		f.size, r = st.Size(), fh
	}
	if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: f.size}); err != nil {
		return err
	}
	n, err := io.Copy(tw, io.LimitReader(r, f.size))
	if err != nil {
		return err
	}
	if n != f.size {
		return fmt.Errorf("%s changed while packing", f.name)
	}
	return nil
}

func fromFiles(files []file, skipped []string) (*Pack, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	p := &Pack{Skipped: skipped}
	for i := range files {
		f := &files[i]
		if err := addFile(tw, f); err != nil {
			return nil, err
		}
		p.Files++
		p.Bytes += f.size
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	if buf.Len() > MaxPublicationBytes {
		return nil, fmt.Errorf("the compressed page is %.0f MB: at most %d MB per publication", float64(buf.Len())/(1<<20), MaxPublicationBytes>>20)
	}
	p.Archive = buf.Bytes()
	return p, nil
}

// badName mirrors hatch-control's path guard so the CLI names the culprit.
func badName(name string) string {
	for _, seg := range strings.Split(name, "/") {
		if len(seg) > 255 {
			return "a path segment is longer than 255 bytes"
		}
	}
	if len(name) > 1024 {
		return "the path is longer than 1024 bytes"
	}
	if strings.Contains(name, "\\") {
		return "backslashes are not allowed in names"
	}
	for _, r := range name {
		if r != '‍' && (unicode.IsControl(r) || unicode.Is(unicode.Cf, r)) {
			return "control or invisible formatting characters are not allowed in names"
		}
	}
	return ""
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify turns a title or file name into a page name candidate:
// "Bilan d'été 2026" -> "bilan-d-ete-2026". "" when nothing usable is left.
func Slugify(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		if unicode.Is(unicode.Mn, r) {
			continue // strip accents
		}
		b.WriteRune(r)
	}
	out := strings.Trim(nonSlug.ReplaceAllString(b.String(), "-"), "-")
	if len(out) > 80 {
		out = strings.TrimRight(out[:80], "-")
	}
	return out
}
