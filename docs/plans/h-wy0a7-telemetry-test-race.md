# Plan — fix data race in `internal/telemetry` test under `-race` (h-wy0a7)

**Bead:** h-wy0a7 (P3, bug). Pre-existing (origin/main) data race found incidentally
while shipping energy packs (h-1qdoo); **not** caused by that change. CI runs
`go test ./... -race`, so this intermittently reds CI.

**Repo:** EscapeVelocityOperations/hatch-cli. **Base:** `main`. **Single PR.**
Projected ≈ 10–15 LOC, **test file only** (`internal/telemetry/telemetry_test.go`).
No production-code change in the primary fix.

> Executor note: the plan doc lives in the meta-repo at
> `/Users/cstar/rigs/hatch/docs/plans/h-wy0a7-telemetry-test-race.md` (same place as
> every hatch plan, incl. the cli `energy-packs.md`). Copy it into your hatch-cli
> branch under `docs/plans/` and commit it with the fix so the PR is self-describing
> (mirrors the h-eh7x2 "this plan doc is in the diff" convention).

## Root cause — two unsynchronized cross-goroutine accesses, "synchronized" only by `time.Sleep`

`time.Sleep` establishes **no happens-before edge**, so the race detector flags
two shared accesses in `TestSendFiresHTTPRequest`:

1. **The `received` struct (the reported race).** The httptest handler goroutine
   writes it — `json.NewDecoder(r.Body).Decode(&received)` at `telemetry_test.go:42`
   — while the test goroutine reads `received.Command/Error/Mode` at lines 56–64.
   Confirmed red:
   ```
   WARNING: DATA RACE
   Read at … by goroutine 8:  …telemetry_test.go:56
   Previous write at … by goroutine 12:  …telemetry_test.go:42 (json Decode → reflect.SetString)
   ```
2. **The `APIHost` package global (the bead's "package-global write").** `Send`'s
   background goroutine reads `host := APIHost` at `telemetry.go:70`, while the test's
   deferred `func() { APIHost = oldHost }()` (registered at `telemetry_test.go:49`)
   writes it on return. The deferred restore runs **before** the deferred
   `server.Close()` (defers are LIFO: line 49 registered after line 45), so nothing
   joins the goroutine before the restore → no edge between the goroutine's read and
   the restore.

The `APIHost` **set** at line 48 is already safe (it precedes the `Send`/goroutine
spawn — program order + go-spawn give the edge). Only the **restore** races.

`TestSendEmptyErrorIsNoop` is **not** racy: empty `errMsg` makes `Send` return
before spawning any goroutine, so the handler never fires and `called`/`APIHost`
are touched by the test goroutine only. Its `time.Sleep(100ms)` is dead weight.

## Fix — replace the sleep with a handler-completion channel (one edge fixes both)

Synchronize on the **handler** finishing, then read. `close(done)` → `<-done` is a
channel happens-before edge:

```go
func TestSendFiresHTTPRequest(t *testing.T) {
	var received Event
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/telemetry" {
			t.Errorf("expected /telemetry, got %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusCreated)
		close(done) // signal: received is fully written
	}))
	defer server.Close()

	oldHost := APIHost
	APIHost = server.URL
	defer func() { APIHost = oldHost }()

	Send("hatch deploy", "--runtime node", "deploy failed: timeout", "cli")

	select {
	case <-done: // deterministic; replaces time.Sleep
	case <-time.After(2 * time.Second):
		t.Fatal("telemetry request not received within 2s")
	}

	// …existing assertions on received.Command/Error/Mode, unchanged…
}
```

**Why this clears both races:**
- `received`: handler write (`:42`) → `close(done)` → `<-done` → test read. Direct
  channel edge — no reliance on net/http internals.
- `APIHost` restore: the handler running means the goroutine already executed
  `host := APIHost` (`telemetry.go:70`) before its `client.Post` returned and the
  server ran the handler. Chain: read(70) → Post(77) → [net/http] → handler →
  `close(done)` → `<-done` → return → deferred restore. So the restore happens-after
  the read. (The Go race detector instruments net/http's internal sync, so the
  Post→handler hop carries the edge.)

Keep the `time.After` guard so a regression deadlocks into a clear `t.Fatal`, not a
hang. Also delete the now-pointless `time.Sleep(100 * time.Millisecond)` in
`TestSendEmptyErrorIsNoop` (same file, one line; the `called` bool stays a plain
read — no goroutine exists to race it).

**Do not** change `telemetry.go` for the primary fix. If — and only if — `-race`
still flags `APIHost` on CI's linux/amd64 (it will not; the edge above is real),
the fallback is a test-only completion hook: an unexported `var afterSend func()`
in `telemetry.go`, invoked in a `defer` at the top of the goroutine, set by the
test to `close(done)` and reset to `nil` on return. Note it under Open questions;
do not pre-emptively add production code.

## Micro-tasks (red → green → commit)

| id | description | acceptance (single failing test) | est_min | slings |
| --- | --- | --- | --- | --- |
| T-001 | **Confirm red.** From the hatch-cli worktree run `go test -race -run TestSendFiresHTTPRequest ./internal/telemetry/`; capture the `WARNING: DATA RACE` (write `:42` vs read `:56`). This is the failing state the fix must clear. | Command exits **non-zero** and prints `DATA RACE` referencing `telemetry_test.go:42`/`:56`. | 3 | — |
| T-002 | In `TestSendFiresHTTPRequest`, add `done := make(chan struct{})`, `close(done)` after the handler's `WriteHeader`, and replace `time.Sleep(100ms)` with a `select { <-done / <-time.After(2s)→t.Fatal }`. Delete the dead `time.Sleep` in `TestSendEmptyErrorIsNoop`. | `go test -race ./internal/telemetry/` is **GREEN** (was red); both tests still pass, assertions unchanged. | 5 | — |
| T-003 | **Verification gate.** | `gofmt -l internal/telemetry` empty; `go vet ./internal/telemetry/`; `go build ./...`; `go test ./... -race` green; `git diff --name-only` ⊆ {`internal/telemetry/telemetry_test.go`} (+ this plan doc under `docs/plans/`). | 4 | — |

Total ≈ 12 min. TDD honored: T-001 pins the red (`-race` is the failing assertion
for a race fix); T-002 makes it green; no production behavior changes.

If `go test ./... -race` (T-003) surfaces a race in a **different** package, that is
pre-existing and **out of scope** — file a follow-up bead, do not expand this PR.

## GDPR data-flow impact

**None.** This is a test-synchronization change. It does not alter what the
telemetry path collects, transmits, or redacts. The `Event` payload (command,
redacted args, redacted error, OS, arch, CLI version, mode) and the redaction
seams (`redact` / `redactArgs`, which strip token/key/password/secret/credential
`k=v` pairs and `api.RedactToken`) are **untouched**. No personal data is added to
logs or tests — assertions read only the fixed strings already in the test. The
fire-and-forget production contract of `Send` is preserved (no signature change).

## MDR Class I traceability

N/A — hatch-cli is the PaaS command-line client, not the voxmemo → voxist-api
clinical documentation pipeline, so MDR Class I chain-of-evidence traceability does
not apply. Heading retained per the `writing-plan` discipline so an auditor sees the
consideration was explicit.

## Open questions (executor / reviewer-resolvable — no PM decision)

- Confirm `go test ./... -race` is the exact CI invocation (the bead states it is);
  if CI scopes `-race` to specific packages, the T-003 gate still covers the fix.
- Fallback only if CI's race detector still flags `APIHost` (not expected): add the
  test-only `afterSend func()` completion hook in `telemetry.go` described above,
  rather than guarding `APIHost` with a mutex (a mutex would touch the prod hot path
  for a test-only concern).
- `close(done)` assumes exactly one handler invocation — true here (`Send` issues a
  single `POST`). If a future change makes the handler fire more than once, switch to
  `sync.Once` around the close.
