// Package sitedeploy deploys a static directory to an egg that serves a Hatch
// site (T-506, h-bgdsq.6): instead of an artifact that becomes an image, it
// sends the list of files (path, sha256, size), uploads only the files the
// platform does not hold yet, then publishes the list as the egg's next
// version. Contract: hatch-api docs/plans/h-bgdsq-9-site-ingest-relay.md.
//
// A redeploy that changes 5 % of the files uploads those files and nothing
// else; the rest of the bytes on the wire are the file lists themselves.
package sitedeploy

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ignore"
)

// Client is the part of the Hatch API a site deploy uses (*api.Client).
type Client interface {
	IngestSiteManifest(slug string, files []api.SiteFile) (*api.SiteIngestResult, error)
	UploadSiteBlobs(slug string, gzippedTar io.Reader) (*api.SiteBlobsResult, error)
	PublishSite(slug string, files []api.SiteFile) (*api.SitePublishResult, error)
}

// ErrSitesDisabled is returned by Publish when the platform answers that
// sites are switched off before anything was sent: the caller deploys an
// artifact instead.
var ErrSitesDisabled = errors.New("hatch sites are not enabled on this platform")

// Manifest is the file list of a deploy target.
type Manifest struct {
	Files    []api.SiteFile
	Bytes    int64    // sum of the file sizes
	Excluded []string // left out by .hatchignore and the safety defaults ("dir/" for a directory)
	Skipped  []string // symlinks that leave the directory, point at a directory or at a file left out, and other non-regular entries

	// source maps a sha256 to a file that holds it, for the upload.
	source map[string]string
}

// Unique returns the number of distinct blobs of the manifest.
func (m *Manifest) Unique() int { return len(m.source) }

// Build lists dir the way an artifact is built from it: .hatchignore (or the
// built-in defaults) decides what is left out, and env files never go. Each
// file is read once to hash it; nothing is kept in memory but the list. A
// symlink to a file inside dir is published with that file's content (the
// artifact's web server followed it) unless that file is itself left out
// (.hatchignore, env file, .git): then the link is skipped, as the artifact
// leaves it dead. Any other symlink is skipped too.
func Build(dir string) (*Manifest, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving directory: %w", err)
	}
	realDir, err := filepath.EvalSymlinks(absDir)
	if err != nil {
		return nil, fmt.Errorf("resolving directory: %w", err)
	}

	matcher, err := ignore.LoadFile(filepath.Join(dir, ".hatchignore"))
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading .hatchignore: %w", err)
		}
		matcher = ignore.DefaultMatcher()
	}
	if negations := matcher.EnvNegations(); len(negations) > 0 {
		return nil, fmt.Errorf(".hatchignore re-includes env files (%s), but env files are never deployed: they hold secrets.\n\n"+
			"Remove those lines from .hatchignore. Set the variables on the egg instead: hatch env set KEY=VALUE\n"+
			"Keep a template as .env.example or .env.sample, which are sent",
			strings.Join(negations, ", "))
	}

	m := &Manifest{source: map[string]string{}}
	byPath := map[string]string{} // normalized path -> original relative path
	// Walk the absolute directory: with a relative --deploy-target
	// (dist), EvalSymlinks would hand back relative paths that never match
	// realDir.
	err = filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == absDir {
			return nil
		}
		rel, err := filepath.Rel(absDir, path)
		if err != nil {
			return err
		}
		if matcher.ShouldExclude(rel, info.IsDir()) {
			if info.IsDir() {
				m.Excluded = append(m.Excluded, rel+"/")
				return filepath.SkipDir
			}
			m.Excluded = append(m.Excluded, rel)
			return nil
		}
		if info.IsDir() {
			return nil
		}

		src := path
		if info.Mode()&os.ModeSymlink != 0 {
			target, ok := symlinkTarget(path, realDir, matcher)
			if !ok {
				m.Skipped = append(m.Skipped, rel)
				return nil
			}
			src = target
		} else if !info.Mode().IsRegular() {
			m.Skipped = append(m.Skipped, rel)
			return nil
		}

		// The platform stores names in NFC (macOS file systems may hand
		// out NFD), with forward slashes.
		name := norm.NFC.String(filepath.ToSlash(rel))
		if prev, dup := byPath[name]; dup {
			return &siteOnlyError{fmt.Sprintf("%q and %q are the same file name once normalized: rename one of them", prev, rel)}
		}
		byPath[name] = rel

		sum, size, err := hashFile(src)
		if err != nil {
			return err
		}
		m.Files = append(m.Files, api.SiteFile{Path: name, Sha256: sum, Size: size})
		m.Bytes += size
		if _, ok := m.source[sum]; !ok {
			m.source[sum] = src
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(m.Files) == 0 {
		return nil, &siteOnlyError{fmt.Sprintf("nothing to publish: %s holds no file (after .hatchignore)", dir)}
	}
	return m, nil
}

// siteOnlyError is a refusal of Build that an artifact deploy of the same
// directory does not make: an empty list, two names equal once normalized.
type siteOnlyError struct{ msg string }

func (e *siteOnlyError) Error() string { return e.msg }

// IsSiteOnly reports whether err is a refusal of Build that only a site deploy
// makes (nothing to publish, names equal once normalized). Any other error of
// Build (.hatchignore, env files, unreadable file) stops an artifact deploy
// as well.
func IsSiteOnly(err error) bool {
	var e *siteOnlyError
	return errors.As(err, &e)
}

// symlinkTarget resolves a symlink to a regular file inside realDir that the
// matcher keeps: the link's own path was checked by the caller, the file it
// ends at is checked here (the file and each directory above it), so a link
// cannot publish what .hatchignore, the env rule or the .git default leave
// out.
func symlinkTarget(path, realDir string, matcher *ignore.Matcher) (string, bool) {
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(realDir, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	st, err := os.Stat(target)
	if err != nil || !st.Mode().IsRegular() {
		return "", false
	}
	if leftOut(matcher, rel) {
		return "", false
	}
	return target, true
}

// leftOut reports whether the matcher excludes the file rel (relative to the
// deploy target, resolved) or one of the directories above it: the walk
// never enters an excluded directory.
func leftOut(matcher *ignore.Matcher, rel string) bool {
	parts := strings.Split(rel, string(filepath.Separator))
	for i := range parts {
		if matcher.ShouldExclude(filepath.Join(parts[:i+1]...), i < len(parts)-1) {
			return true
		}
	}
	return false
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, fmt.Errorf("reading %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// Result is the outcome of a site deploy.
type Result struct {
	Publish *api.SitePublishResult
	// UploadedFiles / UploadedBytes are the blobs sent (file bytes, before
	// compression); SentBytes is what the uploads put on the wire.
	UploadedFiles int
	UploadedBytes int64
	SentBytes     int64
}

// RetryDelay is the pause before the first retry of a transient failure;
// it doubles for the next one. Tests set it to 0.
var RetryDelay = 2 * time.Second

// maxUploads bounds the blob uploads of one deploy: after a transient
// failure the manifest is posted again and only what is still missing is
// sent (blobs stored before the failure are kept by the platform).
const maxUploads = 3

// Publish runs the whole contract on slug: post the manifest, upload the
// missing blobs, post the manifest again until nothing is missing, then
// publish the version. progress, if not nil, receives one line per step.
// ErrSitesDisabled means nothing happened and an artifact deploy should be
// used instead; any other error is a refusal or a failure to report.
func Publish(c Client, slug string, m *Manifest, progress func(string)) (*Result, error) {
	say := func(format string, args ...any) {
		if progress != nil {
			progress(fmt.Sprintf(format, args...))
		}
	}
	res := &Result{}
	pause := func(n int) { time.Sleep(time.Duration(1<<uint(n-1)) * RetryDelay) }

	uploads, manifestFailures := 0, 0
	var lastUploadErr error // the transient failure of the last upload, if any
	first := true
	for {
		ing, err := c.IngestSiteManifest(slug, m.Files)
		if err != nil {
			if first && api.IsSitesDisabled(err) {
				return nil, ErrSitesDisabled
			}
			if transient(err) && manifestFailures < maxUploads {
				manifestFailures++
				pause(manifestFailures)
				continue
			}
			return nil, fmt.Errorf("sending the file list: %w", err)
		}
		if first {
			say("%d files (%s), %d to upload (%s)", len(m.Files), FormatBytes(m.Bytes), len(ing.Missing), FormatBytes(ing.MissingBytes))
		}
		first = false
		if len(ing.Missing) == 0 {
			break
		}
		if uploads == maxUploads {
			if lastUploadErr != nil {
				return nil, fmt.Errorf("uploading files (%d attempts): %w", uploads, lastUploadErr)
			}
			return nil, fmt.Errorf("%d files are still missing on the platform after %d uploads: deploy again", len(ing.Missing), uploads)
		}
		uploads++
		sent, n, bytes, err := upload(c, slug, m, ing.Missing)
		res.SentBytes += sent
		if err != nil {
			if !transient(err) {
				return nil, fmt.Errorf("uploading files: %w", err)
			}
			lastUploadErr = err
			if uploads < maxUploads {
				say("upload interrupted (%v), resuming", err)
				pause(uploads)
			}
			continue // the manifest tells what is still missing
		}
		lastUploadErr = nil
		res.UploadedFiles += n
		res.UploadedBytes += bytes
	}

	pub, err := c.PublishSite(slug, m.Files)
	if err != nil {
		var se *api.SiteError
		if errors.As(err, &se) && se.Status < 500 {
			return nil, fmt.Errorf("publishing the site: %w", err)
		}
		return nil, fmt.Errorf("publishing the site: %w (the version may have been published: deploying again is safe, it uploads nothing and publishes one more version)", err)
	}
	res.Publish = pub
	return res, nil
}

// transient tells the failures worth another round: the connection broke
// (never a client timeout: the upload budget is 30 minutes, and never a
// failure that repeats: unknown host, TLS certificate, refused connection),
// the platform could not reach its storage (502), or the upload was cut.
func transient(err error) bool {
	var se *api.SiteError
	if errors.As(err, &se) {
		return se.Status == 502 || se.Code == "site_upload_interrupted"
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return !ue.Timeout() && !permanentTransport(ue.Err)
	}
	return false
}

// permanentTransport tells the transport failures that a new try will not
// fix: the name does not resolve, the TLS certificate or the address is
// wrong, the connection is refused. A reset, an EOF or a broken pipe on an
// open connection stay transient.
func permanentTransport(err error) bool {
	var dns *net.DNSError
	if errors.As(err, &dns) {
		// A DNS lookup that timed out or failed on the resolver's side may
		// work next time; "no such host" does not.
		return dns.IsNotFound
	}
	var (
		unknownAuthority x509.UnknownAuthorityError
		hostname         x509.HostnameError
		certInvalid      x509.CertificateInvalidError
		recordHeader     tls.RecordHeaderError
		certVerify       *tls.CertificateVerificationError
		addr             *net.AddrError
		parse            *net.ParseError
	)
	switch {
	case errors.As(err, &unknownAuthority), errors.As(err, &hostname), errors.As(err, &certInvalid),
		errors.As(err, &recordHeader), errors.As(err, &certVerify), errors.As(err, &addr), errors.As(err, &parse):
		return true
	}
	return errors.Is(err, syscall.ECONNREFUSED)
}

// upload streams the gzipped tar of the missing blobs: one entry per blob,
// named by its sha256. It returns the bytes on the wire, then the blobs and
// file bytes written into the tar.
func upload(c Client, slug string, m *Manifest, missing []string) (sent int64, files int, bytes int64, err error) {
	pr, pw := io.Pipe()
	type written struct {
		files int
		bytes int64
		err   error
	}
	done := make(chan written, 1)
	counter := &countingWriter{w: pw}
	go func() {
		n, b, err := writeBlobs(counter, m, missing)
		pw.CloseWithError(err) // nil: the reader sees io.EOF
		done <- written{n, b, err}
	}()
	_, upErr := c.UploadSiteBlobs(slug, pr)
	// The API may answer before reading everything: unblock the writer.
	pr.CloseWithError(errors.New("upload finished"))
	w := <-done
	switch {
	case isLocal(w.err):
		return counter.n, 0, 0, w.err // a file of the deploy target could not be read
	case upErr != nil:
		return counter.n, 0, 0, upErr
	case w.err != nil:
		return counter.n, 0, 0, w.err
	}
	return counter.n, w.files, w.bytes, nil
}

// localError marks a failure to read a file of the deploy target.
type localError struct{ err error }

func (e *localError) Error() string { return e.err.Error() }
func (e *localError) Unwrap() error { return e.err }

func isLocal(err error) bool {
	var le *localError
	return errors.As(err, &le)
}

func writeBlobs(w io.Writer, m *Manifest, missing []string) (int, int64, error) {
	sizes := make(map[string]int64, len(m.Files))
	for _, f := range m.Files {
		sizes[f.Sha256] = f.Size
	}
	// Most of a site is already compressed (images, fonts, video) or small:
	// the fastest level keeps the CPU off the critical path of the upload.
	gz, _ := gzip.NewWriterLevel(w, gzip.BestSpeed)
	tw := tar.NewWriter(gz)
	var total int64
	for _, sum := range missing {
		src, ok := m.source[sum]
		if !ok {
			return 0, 0, &localError{fmt.Errorf("the platform asked for a file that is not in the list (%s)", sum)}
		}
		size := sizes[sum]
		if err := tw.WriteHeader(&tar.Header{
			Name:     sum,
			Typeflag: tar.TypeReg,
			Mode:     0o644,
			Size:     size,
			Format:   tar.FormatUSTAR,
		}); err != nil {
			return 0, 0, err
		}
		if err := copyBlob(tw, src, size); err != nil {
			return 0, 0, err
		}
		total += size
	}
	if err := tw.Close(); err != nil {
		return 0, 0, err
	}
	if err := gz.Close(); err != nil {
		return 0, 0, err
	}
	return len(missing), total, nil
}

// copyBlob copies exactly size bytes of src. A file that shrank since it was
// listed is an error here; one whose content changed is refused by the
// platform (site_blob_hash_mismatch).
func copyBlob(w io.Writer, src string, size int64) error {
	f, err := os.Open(src)
	if err != nil {
		return &localError{err}
	}
	defer func() { _ = f.Close() }()
	n, err := io.Copy(w, io.LimitReader(localReader{f}, size))
	if err != nil {
		return err
	}
	if n != size {
		return &localError{fmt.Errorf("%s changed during the deploy (%d bytes, was %d): deploy again", src, n, size)}
	}
	return nil
}

// localReader marks read errors as local, to tell them from a closed upload.
type localReader struct{ r io.Reader }

func (l localReader) Read(p []byte) (int, error) {
	n, err := l.r.Read(p)
	if err != nil && err != io.EOF {
		err = &localError{err}
	}
	return n, err
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// FormatBytes renders n bytes in B, KB, MB or GB (powers of 1024), one decimal.
func FormatBytes(n int64) string {
	const k = 1024
	switch {
	case n < k:
		return fmt.Sprintf("%d B", n)
	case n < k*k:
		return fmt.Sprintf("%.1f KB", float64(n)/k)
	case n < k*k*k:
		return fmt.Sprintf("%.1f MB", float64(n)/(k*k))
	}
	return fmt.Sprintf("%.1f GB", float64(n)/(k*k*k))
}
