# Plan — hatch CLI `volume` command group (h-62x9d)

**Bead:** h-62x9d — ship `hatch volume enable/status/disable` (backend live, CLI never merged).
**Depends on:** h-gcf5h (volumes backend — CLOSED/live in prod since 2026-06-12).
**Date:** 2026-06-25. **Author:** hatch/voxist.planner-1.
**Repo:** hatch-cli (`github.com/EscapeVelocityOperations/hatch-cli`).
**Branch:** `gc/h-62x9d` (off `origin/main`). **PR:** single PR, this repo only.

---

## Problem statement

The persistent-volumes feature (h-gcf5h) shipped its API + control plane to prod
on 2026-06-12 and is red-team certified. The **user-facing CLI verb was never
merged**: `main` has no `cmd/volume/`, and `internal/api/client.go` /
`internal/api/types.go` contain zero volume references. Two branches
(`gc/h-g91eg`, `gc/h-poo35`) and commit `440fcda` hold an `errNotImplemented`
stub + a green attempt, but the bead `h-g91eg` was closed without the code ever
landing on `main`. Volumes are therefore **API-only and undiscoverable** to CLI
users. This plan ships the real client + the `volume` command, registers it, and
fronts every change with a failing test.

## What ships in this bead

`hatch volume enable | status | disable`, the three matching `*api.Client`
methods, the `api.Volume` type, and root-command registration — with happy-path
+ contract tests. Landing-site docs are a **separate repo / separate PR** —
tracked as a fast-follow bead (see "Landing docs decision").

---

## Reference material — salvage, but DO NOT cherry-pick blind

The stranded refs are a useful starting point, but **the stub's type is wrong**
and this plan supersedes its signatures:

- `git show gc/h-poo35:cmd/volume/volume_test.go` — 6 red TDD tests (structure,
  auth, param passing, grace default, `--now`). Good *shape*; outdated
  *signatures* (see below). Port the intent, not the literal funcs.
- `git show 440fcda:cmd/volume/volume.go` — green command impl following the
  `domain` pattern. Usable as a skeleton.
- `git show 440fcda:internal/api/client.go` / `:internal/api/types.go` — the
  stranded client + type.

### ⚠️ Correctness defect in the stranded stub — must be fixed

The stranded `Volume` type **omits two fields** the live API returns, so a
verbatim revival would ship a `status` command that silently drops the
over-quota warning and the grace-deletion deadline:

```go
// STRANDED (440fcda) — WRONG, missing DeleteAfter + OverQuota:
type Volume struct {
    SizeMB int    `json:"size_mb"`
    UsedMB int    `json:"used_mb"`
    Mount  string `json:"mount"`
    Status string `json:"status"`
}
```

The live API (`hatch-api/internal/api/handlers/volumes.go:169`) returns:

```go
type volumeResponse struct {
    SizeMB      int32  `json:"size_mb"`
    UsedMB      int32  `json:"used_mb"`
    Status      string `json:"status"`        // active | grace_deleting | deleted
    Mount       string `json:"mount"`         // always "/data"
    DeleteAfter string `json:"delete_after,omitempty"` // RFC3339, set when grace_deleting
    OverQuota   bool   `json:"over_quota"`     // used_mb > size_mb
}
```

The CLI `Volume` type **must** carry `DeleteAfter` and `OverQuota`. `int` (not
`int32`) is fine — JSON unmarshals the numbers into `int` and the CLI only
renders them. The contract test in T-003 exists specifically to catch this
omission.

---

## API contract (source of truth — `hatch-api`, live)

Base: `https://api.gethatch.eu` + `/v1`. Auth: `Authorization: Bearer <token>`
(standard app-scoped owner token, same as every other `/apps/{slug}/…` call).
The client's existing `do()` already treats any `>= 400` as an error, so the
422/404/409 cases surface as `API error <code>: <body>` without extra work.

| METHOD | PATH | Request JSON | Success | Response JSON | Errors |
| --- | --- | --- | --- | --- | --- |
| POST | `/v1/apps/{slug}/volume` | `{"size_mb": <int>}` | **201** new / **200** reactivate | `volumeResponse` | 400 body · 401 auth · 404 app · 409 already-active · **422** size ≤0/over-cap · 500 |
| GET | `/v1/apps/{slug}/volume` | — | **200** | `volumeResponse` | 401 · 404 app/no-volume · 500 |
| DELETE | `/v1/apps/{slug}/volume[?now=true]` | — | **202** | `volumeResponse` (with `delete_after`) | 401 · 404 app/no-active-volume · 500 |

Notes that shape CLI behavior:

- **`size_mb` is required by the API (no server default).** The CLI defaults
  `--size` to **1024** (the free-tier cap) when omitted, matching the h-gcf5h
  spec `hatch volume enable [--size 1024]`. Over-tier-cap (free 1024 / pro 5120
  MB) → API `422 {"error":"size_mb must be between 1 and the tier cap"}`; the
  CLI surfaces that message verbatim.
- **Disable returns `202` + body** including `delete_after`. Default = 7-day
  grace; `?now=true` = immediate (`delete_after` ≈ now). Decoding the body lets
  the CLI print the exact deadline rather than a vague "scheduled".
- The `/internal/...volume...` routes are control-plane only — **the CLI never
  touches them**.

---

## Design decisions

1. **Client method signatures — all return `(*api.Volume, error)`.** Every
   endpoint returns the `volumeResponse` body, so decoding it on all three lets
   the command echo real server state (provisioned size + mount on enable, exact
   `delete_after` on disable). This is house style for body-returning calls
   (cf. `AddDomain` → `(*Domain, error)`), and it **supersedes the stranded
   red-test signatures** (which used `error`-only enable/disable + a value
   `VolumeInfo`):

   ```go
   func (c *Client) EnableVolume(slug string, sizeMB int) (*Volume, error)  // POST, decode 201/200
   func (c *Client) GetVolume(slug string)              (*Volume, error)    // GET,  decode 200
   func (c *Client) DisableVolume(slug string, now bool)(*Volume, error)    // DELETE[?now], decode 202
   ```

   Each calls `validateSlug(slug)` first (house guard), builds the path as
   `"/apps/"+slug+"/volume"`, and `EnableVolume` sends `{"size_mb":N}` via
   `fmt.Sprintf` like `AddDomain` does. `DisableVolume` appends `?now=true` only
   when `now`.

2. **Command shape mirrors `cmd/domain`** (the closest CRUD analog): a `Deps`
   struct of injectable funcs, `defaultDeps()` wiring `api.NewClient(token).X()`,
   a `NewCmd()` aggregating `enable`/`status`/`disable`, an `-a/--app` flag, and
   a `resolveSlug` helper reused from the domain pattern. Cmd `Deps`:

   ```go
   type Deps struct {
       GetToken      func() (string, error)
       EnableVolume  func(token, slug string, sizeMB int) (*api.Volume, error)
       GetVolume     func(token, slug string)            (*api.Volume, error)
       DisableVolume func(token, slug string, now bool)  (*api.Volume, error)
   }
   ```

3. **Rendering.** `status` prints size / used / mount / status; appends an
   **over-quota warning** when `OverQuota`, and the **grace deadline**
   (`delete_after`) when `status == grace_deleting`. `enable` confirms size +
   mount. `disable` prints the grace deadline (or "deleting now" with `--now`),
   read from the returned body.

4. **Tests at two layers.** The cmd-layer DI mocks (house pattern) cannot catch
   JSON-tag / contract drift because they bypass the client — so the contract is
   pinned by **httptest client tests** (`internal/api/client_test.go`, the
   existing `c.host = server.URL` idiom). The over-quota/grace client test
   (T-003) is the one that would have caught the stranded type's omission.

## Files to change

| File | Change |
| --- | --- |
| `internal/api/types.go` | add `Volume` struct (6 fields incl. `DeleteAfter`, `OverQuota`) |
| `internal/api/client.go` | add `EnableVolume` / `GetVolume` / `DisableVolume` |
| `internal/api/client_test.go` | add 3 httptest contract tests |
| `cmd/volume/volume.go` | **new** — command group + `Deps` + `defaultDeps` + run bodies |
| `cmd/volume/volume_test.go` | **new** — DI cmd tests (ported + extended) |
| `cmd/root/root.go` | register `rootCmd.AddCommand(volume.NewCmd())` + import |

---

## Micro-tasks (TDD — red before green)

| id | description | acceptance (one failing test) | est_min | slings |
| --- | --- | --- | --- | --- |
| T-001 | Write failing httptest for `EnableVolume` | `TestEnableVolume` in `client_test.go`: server asserts `POST /v1/apps/demo/volume` + body `{"size_mb":2048}` + `Bearer tok`, returns `201` + `volumeResponse`; test asserts decoded `*Volume{SizeMB:2048,Mount:"/data",Status:"active"}`. Fails to compile (no method). | 4 | — |
| T-002 | Add `Volume` type + `EnableVolume` | T-001 passes. `Volume` has all 6 fields incl. `DeleteAfter`,`OverQuota`. | 4 | — |
| T-003 | Write failing httptest for `GetVolume` over-quota + grace | `TestGetVolume_GraceAndOverQuota`: server returns `200` + body `{...,"status":"grace_deleting","delete_after":"2026-07-02T18:30:45Z","over_quota":true}`; asserts client decodes `DeleteAfter` **and** `OverQuota` (guards the stub defect). RED. | 4 | — |
| T-004 | Implement `GetVolume` | T-003 passes. | 3 | — |
| T-005 | Write failing httptest for `DisableVolume` grace-vs-now | `TestDisableVolume`: default call → server sees `DELETE /v1/apps/demo/volume` (no query); `now=true` → `?now=true`; server returns `202`+body; asserts decoded `delete_after`. RED. | 5 | — |
| T-006 | Implement `DisableVolume` | T-005 passes. | 3 | — |
| T-007 | Write failing cmd structure + enable-default test | `TestNewCmd_Structure` (volume → enable/status/disable) + `TestRunEnable_DefaultsSizeTo1024` (omit `--size` ⇒ client called with `1024`) + `TestRunEnable_NotLoggedIn` (empty token ⇒ "not logged in" error). RED (no package). | 5 | — |
| T-008 | Implement `cmd/volume/volume.go` enable + wiring | T-007 passes: `NewCmd`, `Deps`, `defaultDeps`, `-a/--app`, `--size` (default 1024), `runEnable`. | 5 | — |
| T-009 | Write failing `status` render test | `TestRunStatus_RendersOverQuotaAndGrace`: mock `GetVolume` returns over-quota grace volume; captured stdout contains used/size, an over-quota warning, and the `delete_after` value. RED. | 4 | — |
| T-010 | Implement `runStatus` | T-009 passes. | 4 | — |
| T-011 | Write failing `disable` test | `TestRunDisable_GraceByDefault` + `TestRunDisable_NowFlag`: assert `now` bool passed through and output reflects grace deadline vs immediate. RED. | 4 | — |
| T-012 | Implement `runDisable` | T-011 passes. | 3 | — |
| T-013 | Register in root | add import + `rootCmd.AddCommand(volume.NewCmd())` in `cmd/root/root.go`; `go build ./...` succeeds and `hatch volume --help` lists the three subcommands. | 3 | — |
| T-014 | Full gate | `make test` and `make lint` both green across the repo; no other package regressed. | 4 | — |

No cross-rig slings: the work is entirely within hatch-cli against the
already-live API.

## Open questions

- [executor] `enable` default size: plan sets **1024 MB** (free-tier cap, per
  h-gcf5h spec). If the user is on a pro tier they pass `--size` explicitly;
  over-cap is the API's 422 to surface, not the CLI's to pre-validate. Confirm
  no client-side cap check is wanted (keep the API as the single source of
  truth for tier limits).
- [executor] Confirm `resolveSlug` is reusable as-is from `cmd/domain`, or
  factor a shared helper if it's currently unexported there (don't duplicate).
- [reviewer] `disable` UX: print the raw RFC3339 `delete_after`, or humanize
  ("in 7 days")? Plan ships raw timestamp (unambiguous, no tz library); flag if
  product wants friendlier copy.

## GDPR data-flow impact

### Data added / removed / relocated
This bead adds **no new data store** — it is the user-facing trigger for paths
that already exist server-side. It is, however, the **CLI entry point to a
GDPR Art. 17 erasure**: `hatch volume disable --now` instructs the API to set
`delete_after ≈ now` and write a `volume_erasures` audit row (record-then-act,
`hatch-api` F5-A), after which app data at rest in `/var/lib/hatch/volumes/…`
is physically removed. Without `--now`, a 7-day grace applies. The `status`
command now also surfaces `delete_after`, so a data subject can see exactly when
their persisted data is scheduled to be erased.

### New cross-border transfers
None. The CLI calls the existing EU-region API; no data leaves the established
region and the CLI persists nothing locally (no volume payload touches the
user's disk — only the auth token already stored by `hatch login`).

### Audit-log changes
None added by the CLI. The erasure audit row is written by the API, not the
client; the CLI only triggers it. The reviewer should confirm the `disable`
help text makes the grace-vs-`--now` (recoverable-vs-irreversible) distinction
explicit so the destructive path is not invoked by accident.

## MDR Class I traceability

Not applicable — the hatch CLI volume verb is platform tooling, not part of the
voxmemo→voxist-api clinical capture/transcription path. No chain-of-evidence
metadata is created, consumed, or relied upon here. Heading retained for the
auditor's explicit-consideration record.

## Landing docs decision

The re-review asked whether to also ship hatch-landing docs. Decision: **yes,
but as a separate bead/PR** — hatch-landing is a different repo, and folding it
in would break the one-repo/one-PR rule the executor enforces. The CLI's own
`--help` text provides in-CLI discovery immediately; the marketing/guide page is
the remaining gap. A fast-follow bead (created with this plan, **blocked on
h-62x9d** so it documents only what has shipped) covers the "Persistent storage"
guide + CLI/API reference on hatch-landing, routed to `hatch/voxist.executor`.

## Out of scope (v1)

Multiple volumes / custom mount points, client-side quota pre-checks, humanized
deadline copy, any `/internal/*` interaction, and the landing-site guide (its
own bead).
