# h-abmr: `hatch protect` password protection (S2 of egg password-protection)

Bead: h-abmr. Parent scope: h-01ml (superseded by h-macc for enforcement).
Executor-authored plan (bead explicitly delegates decomposition: "executor
writes the micro-tasks TDD").

## Spec (from bead, condensed)

- `hatch protect --password <p>` — enable password protection.
- `hatch protect --off` — disable.
- `hatch protect` (no flags) — show status.
- PASSWORD-ONLY — no `--user` flag (no Basic Auth username concept).
- Never echo the stored password back.
- Honest output — do not overclaim enforcement state.

## Design decision: app resolution

Bead's illustrative syntax (`hatch protect <app> --password <p>`) reads as an
explicit positional app argument. Every existing hatch-cli resource command
(`domain`, `webhook`, `protect email`) instead resolves the app slug from the
current directory's `.hatch.toml` via `resolve.SlugFromDir`, with zero
positional app args. Bead also says "Mirror cmd/domain + cmd/webhook
structure" — that instruction is the more authoritative one (concrete +
structural) vs. the loose example invocation. Following the established
convention for consistency: `hatch protect --password <p>` run from the app
directory, no `<app>` positional. Flagging this as a deliberate deviation
from the bead's literal example text, not an oversight.

## Design decision: honest-output wording

Bead was filed before h-macc (the enforcing auth-gateway) shipped. h-macc's
PR-2/PR-3 (hatch-control #66, #67) merged 2026-07-08. Original plan reasoned
the gateway now unconditionally gates every protected-egg request and had
the CLI print "password set for `<slug>`; enforced by the Hatch
auth-gateway."

**Reviewer REWORK (2026-07-12, fd-111496867c16):** that claim is false
against current origin/main — h-7lbm (P0, fix PR#79 open/unmerged) and
h-wvzu (fix PR#81 open/unmerged) mean protected eggs have documented
zero-auth windows on every wake, boost, deploy, and restart; h-0ed4 (P1,
deferred) means the wakeproxy fails open on a protection-lookup error. The
gate chokepoints from those fixes are absent from hatch-control origin/main.
Asserting "enforced by" is exactly the misleading security half-ship the
bead's HONEST OUTPUT requirement forbids — and reverting to the bead's
original "becomes active once deployed" wording would be false in the
*other* direction (the gateway is deployed, just gap-ridden). Corrected
output states only what's verifiably true and makes no enforcement
guarantee either way: "Password protection enabled for `<slug>`."

## API surface (already live on hatch-api main — verified, no work needed there)

- `POST   /v1/apps/{slug}/protect` `{"password":"..."}` → `{"protected":true}`
- `DELETE /v1/apps/{slug}/protect` → `{"protected":false}`
- `GET    /v1/apps/{slug}/protect` → `{"protected":<bool>}`

(`internal/api/handlers/protect.go`, router.go:282-284.)

## Files to add

- `internal/api/password_protection.go` (+ `_test.go`) — mirrors
  `internal/api/email_protection.go` exactly (same `c.do()` pattern, same
  error wrapping, same httptest conventions).
- `cmd/protect/password.go` (+ `_test.go`) — mirrors `cmd/protect/email.go`'s
  `Deps`/interface/adapter shape, but wires flags onto the existing `protect`
  parent command (`cmd/protect/protect.go`) instead of a verb-subcommand
  group, per the design decision above.
- `cmd/protect/protect.go` — add `RunE` + `--password`/`--off` flags to the
  existing group command.

## Micro-tasks

- [x] T-001 — api client: `SetPasswordProtection` (POST). ✅ green at
  `TestSetPasswordProtection` — commit f81a9e7.
- [x] T-002 — api client: `GetPasswordProtection` (GET). ✅ green at
  `TestGetPasswordProtection` — commit 20ae98d.
- [x] T-003 — api client: `DeletePasswordProtection` (DELETE). ✅ green at
  `TestDeletePasswordProtection` — commit 9f242fc.
- [x] T-004 — api client: error surfacing. ✅ green at
  `TestSetPasswordProtection_APIError` (folded into T-001's commit
  f81a9e7 — same file/cycle).
- [x] T-005 — cmd scaffolding + enable path. ✅ green at
  `TestRunProtectEnable_PostsPassword` — commit 38f2871.
- [x] T-006 — disable path. ✅ green at `TestRunProtectDisable_CallsClear`
  — commit 83d4839.
- [x] T-007 — status path (bare `hatch protect`). ✅ green at
  `TestRunProtectStatus_{Protected,Unprotected}` — commit cbd4256.
- [x] T-008 — validation guards. ✅ green at
  `TestRunProtect_{MutuallyExclusiveFlags,EmptyPassword}` — commit 44c9787
  (includes a small refactor: single flag read at the top of `runProtect`
  instead of re-reading `--password` in the enable branch).

## Acceptance — VERIFIED

`go build ./...` clean, `go test ./...` green (full repo, all packages),
`go vet ./...` clean, `TestProtectCommandRegistered`
(cmd/root/protect_test.go) passes unmodified (no new subcommand registered
— flags added to the existing `protect` command).

## Rework — fd-111496867c16 (2026-07-12)

- [x] Drop "enforced by the Hatch auth-gateway" enforcement claim from the
  enable-path success output (see design-decision update above). ✅ green at
  `TestRunProtectEnable_PostsPassword` — commit 84acc17. Re-verified:
  `go build ./...` clean, `go test ./...` 711 passed / 44 packages,
  `go vet ./...` clean.
