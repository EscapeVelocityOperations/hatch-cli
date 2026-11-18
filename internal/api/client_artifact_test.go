package api

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// virtualArtifact is an io.ReadSeeker of size bytes that exist nowhere: it
// generates 'x' on Read, so a test can upload "999 MiB" without holding it.
// Reads counts the bytes handed out, so a test can see how often the whole
// artifact was read.
type virtualArtifact struct {
	size  int64
	pos   int64
	reads int64
}

var xBlock = bytes.Repeat([]byte{'x'}, 64<<10)

func (v *virtualArtifact) Read(b []byte) (int, error) {
	if v.pos >= v.size {
		return 0, io.EOF
	}
	if rest := v.size - v.pos; int64(len(b)) > rest {
		b = b[:rest]
	}
	n := copy(b, xBlock)
	v.pos += int64(n)
	v.reads += int64(n)
	return n, nil
}

func (v *virtualArtifact) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		v.pos = offset
	case io.SeekCurrent:
		v.pos += offset
	case io.SeekEnd:
		v.pos = v.size + offset
	}
	return v.pos, nil
}

// fastRetries removes the real backoff between upload attempts.
func fastRetries(t *testing.T) {
	t.Helper()
	old := artifactRetryDelay
	artifactRetryDelay = time.Millisecond
	t.Cleanup(func() { artifactRetryDelay = old })
}

type uploadSeen struct {
	mu            sync.Mutex
	contentLength []int64
	received      []int64
	header        http.Header
}

// uploadServer answers each attempt with statuses[i] (the last one repeats)
// after draining and counting the body.
func uploadServer(t *testing.T, statuses ...int) (*Client, *uploadSeen) {
	t.Helper()
	seen := &uploadSeen{}
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ := io.Copy(io.Discard, r.Body)
		seen.mu.Lock()
		seen.contentLength = append(seen.contentLength, r.ContentLength)
		seen.received = append(seen.received, got)
		seen.header = r.Header.Clone()
		seen.mu.Unlock()
		i := int(n.Add(1)) - 1
		if i >= len(statuses) {
			i = len(statuses) - 1
		}
		w.WriteHeader(statuses[i])
		if statuses[i] >= 400 {
			_, _ = io.WriteString(w, "server says no")
		}
	}))
	t.Cleanup(srv.Close)
	c := NewClient("tok123")
	c.host = srv.URL
	return c, seen
}

// TestUploadArtifact_StreamsTheArtifactWithItsContentLength is the h-5ghe9 core
// on the client: 999 MiB goes out with an exact Content-Length, nothing like
// 999 MiB is allocated for it, and the artifact is read once (not once to
// buffer and once to send).
func TestUploadArtifact_StreamsTheArtifactWithItsContentLength(t *testing.T) {
	if testing.Short() {
		t.Skip("moves 999 MiB over loopback")
	}
	c, seen := uploadServer(t, http.StatusOK)
	art := &virtualArtifact{size: 999 * mib}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	err := c.UploadArtifact("myapp", art, "static", "")
	runtime.ReadMemStats(&after)

	if err != nil {
		t.Fatal(err)
	}
	if len(seen.received) != 1 || seen.received[0] != 999*mib || seen.contentLength[0] != 999*mib {
		t.Fatalf("server saw Content-Length %v, body %v; want one request of exactly %d", seen.contentLength, seen.received, 999*mib)
	}
	if art.reads != 999*mib {
		t.Errorf("artifact read %d bytes, want %d (once, not copied into memory first)", art.reads, 999*mib)
	}
	alloc := int64(after.TotalAlloc - before.TotalAlloc)
	t.Logf("uploaded %d MiB, allocated %d KiB", 999, alloc>>10)
	if alloc > 64*mib {
		t.Errorf("allocated %d MiB uploading 999 MiB: the artifact is being buffered", alloc/mib)
	}
	if got := seen.header.Get("X-Artifact-Metadata"); !strings.Contains(got, `"runtime":"static"`) {
		t.Errorf("X-Artifact-Metadata = %q", got)
	}
	if got := seen.header.Get("Authorization"); got != "Bearer tok123" {
		t.Errorf("Authorization = %q", got)
	}
}

// TestUploadArtifact_BytesReaderIsNotCopied: the callers pass a *bytes.Reader;
// sending it must not copy it (that doubled the memory of a 1 GiB tarball).
func TestUploadArtifact_BytesReaderIsNotCopied(t *testing.T) {
	c, seen := uploadServer(t, http.StatusOK)
	data := bytes.Repeat([]byte("artifact!"), 8<<20) // 72 MB, allocated before measuring

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	err := c.UploadArtifact("myapp", bytes.NewReader(data), "node", "node server.js")
	runtime.ReadMemStats(&after)

	if err != nil {
		t.Fatal(err)
	}
	if seen.received[0] != int64(len(data)) || seen.contentLength[0] != int64(len(data)) {
		t.Fatalf("server saw Content-Length %d, body %d; want %d", seen.contentLength[0], seen.received[0], len(data))
	}
	if alloc := int64(after.TotalAlloc - before.TotalAlloc); alloc > int64(len(data))/4 {
		t.Errorf("allocated %d MiB uploading a %d MiB bytes.Reader: it was copied", alloc/mib, int64(len(data))/mib)
	}
}

// TestUploadArtifact_SendsFromTheReadersCurrentPosition: a reader already
// partly consumed uploads only what is left, and retries restart there.
func TestUploadArtifact_SendsFromTheReadersCurrentPosition(t *testing.T) {
	fastRetries(t)
	c, seen := uploadServer(t, http.StatusBadGateway, http.StatusOK)
	r := bytes.NewReader([]byte("HEADER-skipped|the artifact"))
	if _, err := r.Seek(int64(len("HEADER-skipped|")), io.SeekStart); err != nil {
		t.Fatal(err)
	}

	if err := c.UploadArtifact("myapp", r, "node", "node server.js"); err != nil {
		t.Fatal(err)
	}
	want := int64(len("the artifact"))
	if len(seen.received) != 2 || seen.received[0] != want || seen.received[1] != want {
		t.Fatalf("attempts received %v bytes, want two of %d", seen.received, want)
	}
}

// TestUploadArtifact_NonSeekableReaderIsBufferedOnce: a reader that cannot seek
// (a pipe, a gzip stream) still works, read into memory once and re-sent whole.
func TestUploadArtifact_NonSeekableReaderIsBufferedOnce(t *testing.T) {
	fastRetries(t)
	c, seen := uploadServer(t, http.StatusServiceUnavailable, http.StatusOK)
	payload := strings.Repeat("stream", 1000)

	// io.MultiReader hides the Seek method of what it wraps.
	if err := c.UploadArtifact("myapp", io.MultiReader(strings.NewReader(payload)), "node", "node server.js"); err != nil {
		t.Fatal(err)
	}
	if len(seen.received) != 2 || seen.received[0] != int64(len(payload)) || seen.received[1] != int64(len(payload)) {
		t.Fatalf("attempts received %v bytes, want two of %d", seen.received, len(payload))
	}
	if seen.contentLength[0] != int64(len(payload)) {
		t.Errorf("Content-Length = %d, want %d", seen.contentLength[0], len(payload))
	}
}

// TestUploadArtifact_RetryResendsTheWholeArtifact: after a 502 the second
// attempt must send every byte again (rewound), not what was left over.
func TestUploadArtifact_RetryResendsTheWholeArtifact(t *testing.T) {
	fastRetries(t)
	c, seen := uploadServer(t, http.StatusBadGateway, http.StatusBadGateway, http.StatusOK)
	art := &virtualArtifact{size: 5 * mib}

	if err := c.UploadArtifact("myapp", art, "node", "node server.js"); err != nil {
		t.Fatal(err)
	}
	if len(seen.received) != 3 {
		t.Fatalf("%d attempts, want 3", len(seen.received))
	}
	for i, got := range seen.received {
		if got != 5*mib || seen.contentLength[i] != 5*mib {
			t.Errorf("attempt %d: Content-Length %d, body %d, want %d", i+1, seen.contentLength[i], got, 5*mib)
		}
	}
}

// TestUploadArtifact_InsufficientStorageIsNotRetried: 507 means the build host
// is out of disk; uploading the artifact again cannot help.
func TestUploadArtifact_InsufficientStorageIsNotRetried(t *testing.T) {
	fastRetries(t)
	c, seen := uploadServer(t, http.StatusInsufficientStorage)

	err := c.UploadArtifact("myapp", bytes.NewReader([]byte("artifact")), "node", "node server.js")

	if err == nil || !strings.Contains(err.Error(), "upload failed (507)") || !strings.Contains(err.Error(), "server says no") {
		t.Fatalf("error = %v, want upload failed (507) with the server's message", err)
	}
	if len(seen.received) != 1 {
		t.Fatalf("%d attempts for a 507, want exactly 1", len(seen.received))
	}
}

// TestUploadArtifact_ClientTimeoutIsNotRetried: hatch answers only after the
// image is built, so a client-side timeout leaves the server possibly still
// building. A retry would upload the artifact again and start a second build.
func TestUploadArtifact_ClientTimeoutIsNotRetried(t *testing.T) {
	fastRetries(t)
	oldTimeout := artifactDeployTimeout
	artifactDeployTimeout = 150 * time.Millisecond
	t.Cleanup(func() { artifactDeployTimeout = oldTimeout })

	var attempts atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		select { // the docker build that outlasts the client
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	c := NewClient("tok123")
	c.host = srv.URL

	err := c.UploadArtifact("myapp", bytes.NewReader([]byte("artifact")), "node", "node server.js")

	if err == nil || !strings.Contains(err.Error(), "upload timed out after") {
		t.Fatalf("error = %v, want an upload timed out error", err)
	}
	for _, want := range []string{"may still be building", "hatch info myapp"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not say %q", err, want)
		}
	}
	if n := attempts.Load(); n != 1 {
		t.Fatalf("%d attempts after a client-side timeout, want exactly 1", n)
	}
}

// TestUploadArtifact_ConnectionRefusedIsStillRetried: nothing was sent, so
// trying again is safe and the old behaviour stays.
func TestUploadArtifact_ConnectionRefusedIsStillRetried(t *testing.T) {
	fastRetries(t)
	srv := httptest.NewServer(http.NotFoundHandler())
	host := srv.URL
	srv.Close()
	var dials atomic.Int32
	c := NewClient("tok123")
	c.host = host
	tr := c.httpClient.Transport.(*http.Transport)
	orig := tr.DialContext
	tr.DialContext = func(ctx2 context.Context, network, addr string) (net.Conn, error) {
		dials.Add(1)
		return orig(ctx2, network, addr)
	}

	err := c.UploadArtifact("myapp", bytes.NewReader([]byte("artifact")), "node", "node server.js")

	if err == nil {
		t.Fatal("expected an error for a refused connection")
	}
	if n := dials.Load(); n != 3 {
		t.Fatalf("%d connection attempts, want 3", n)
	}
}

// TestUploadArtifact_413IsNotRetriedAndNamesTheLimit: the server's 413 (an
// artifact over the limit) is a 4xx like any other: no retry, and its text,
// which names the limit, reaches the user.
func TestUploadArtifact_413IsNotRetriedAndNamesTheLimit(t *testing.T) {
	fastRetries(t)
	c, seen := uploadServer(t, http.StatusRequestEntityTooLarge)

	err := c.UploadArtifact("myapp", bytes.NewReader([]byte("artifact")), "node", "node server.js")

	if err == nil || !strings.Contains(err.Error(), "upload failed (413)") || !strings.Contains(err.Error(), "server says no") {
		t.Fatalf("error = %v, want upload failed (413) with the server's text", err)
	}
	if len(seen.received) != 1 {
		t.Fatalf("%d attempts for a 413, want 1", len(seen.received))
	}
}
