# Plan: sign + notarize darwin release binaries [h-6ewk]

**Bead:** h-6ewk (task, P3) — from skipper's 2026-07-04 clean-config E2E of the ADR-0023 funnel.
**Owning code:** hatch-cli — `.github/workflows/release.yml`, `scripts/` (new workflow-guard test), `Makefile`, `.goreleaser.yml` (delete).
**Route after planning:** hatch/voxist.executor.
**Ops companion (NOT a blocker):** h-jvc1 — Eric provisions Apple Developer creds → `gh secret set` (human-parked). Code track ships first; signing auto-activates when secrets land.

## Problem

`release.yml` builds darwin-amd64/arm64 with bare `go build` on ubuntu and uploads
unsigned Mach-O binaries. Two user-facing breaks:

1. **App-firewall silent deny (observed live, Little Snitch silent-deny mode):** an
   unsigned binary has no stable code identity, so LS cannot attribute/whitelist it —
   first outbound to `api.gethatch.eu:443` dropped → `i/o timeout` with zero visible
   cause. This breaks the primary bootstrap funnel (`hatch-mcp.sh` → install → MCP).
2. **Gatekeeper quarantine on manual browser downloads:** browser sets the quarantine
   xattr; unsigned+un-notarized binary gets blocked. (The curl path in `install.sh`
   sets no quarantine xattr — signing fixes it via LS identity, not Gatekeeper.)

Fix: Developer ID codesign + notarize in `release.yml`, gated to skip gracefully
(warning, unsigned upload = today's behavior) until the Apple secrets exist.

## Design decisions (pinned here — do not re-litigate in-executor)

1. **darwin matrix legs move to a macOS runner; native toolchain** (`codesign`,
   `xcrun notarytool`). Repo is PUBLIC → macOS runners are free. Third-party
   cross-signing (rcodesign on ubuntu) rejected: extra pinned dependency and a
   notarization reimplementation vs Apple's canonical tool. Implementation: add
   `runner` to each matrix include (`ubuntu-latest` / `macos-latest`), job
   `runs-on: ${{ matrix.runner }}`. linux legs stay on ubuntu.
2. **Sign + notarize as steps inside the darwin build legs**, gated at STEP level:
   `if: ${{ secrets.APPLE_CERT_P12 != '' }}`. The `secrets` context is legal in
   step-level `if` but NOT job-level `if` — do not try to gate the job.
   Secrets absent → an explicit `::warning::release is UNSIGNED (Apple secrets not
   configured — see h-jvc1)` step runs instead, and the unsigned binary uploads
   exactly as today. No behavior change until creds land.
3. **Hardened runtime is mandatory for notarization:** `codesign --sign <identity>
   --options runtime --timestamp`. Plain Go binaries are compatible (no JIT).
4. **No staple — documented deviation from the bead text.** `stapler` only staples
   .app/.dmg/.pkg containers; a bare Mach-O (and a zip) cannot carry a stapled
   ticket. We notarize a zip container of the binary; Gatekeeper resolves the
   ticket ONLINE for browser downloads, which closes the observed failure mode.
   Switching distribution to .dmg/.pkg just to staple would break `install.sh`,
   `hatch-mcp.sh` bootstrap, and the checksum contract — out of scope. Note the
   offline-first-launch limitation in README.
5. **checksums.txt needs no change:** the release job computes sha256 AFTER
   downloading artifacts, so it hashes the signed binaries automatically.
   `install.sh` verifies against that same checksums.txt → untouched. Asset names
   unchanged (`hatch-darwin-{amd64,arm64}`) — the gethatch.eu install endpoint
   keeps working.
6. **Delete the orphaned goreleaser path:** `.goreleaser.yml` is not wired into any
   workflow (confirmed), has no signing config, and references never-shipped
   windows builds + a homebrew tap. `Makefile` `release`/`release-snapshot`
   targets are its only references. Dead config already cost one executor
   investigation (see bead comments) — delete both (git history preserves them;
   resurrect from history if a goreleaser+homebrew route is ever chosen).

## Secrets contract (provisioned by h-jvc1, consumed here)

| Secret | Content |
| --- | --- |
| `APPLE_CERT_P12` | base64 of Developer ID Application cert+key (.p12) |
| `APPLE_CERT_PASSWORD` | .p12 password |
| `APPLE_TEAM_ID` | 10-char team ID |
| `APPLE_NOTARY_KEY_ID` | App Store Connect API key ID |
| `APPLE_NOTARY_ISSUER_ID` | App Store Connect issuer UUID |
| `APPLE_NOTARY_KEY_P8` | contents of the AuthKey .p8 |

App Store Connect API key auth chosen over Apple-ID+app-specific-password (no 2FA
interaction, revocable, notarytool-native: `--key/--key-id/--issuer`).

## Micro-tasks (TDD, red→green)

| id | description | acceptance (failing test → pass) | est_min | slings |
|----|-------------|----------------------------------|---------|--------|
| T-001 | Add failing workflow-guard test `scripts/release_workflow_test.go` (house pattern: `package scripts`, parse `.github/workflows/release.yml` with `gopkg.in/yaml.v3`) asserting: (a) darwin matrix legs declare a macOS runner and job `runs-on` uses `matrix.runner`; (b) a codesign step exists containing `--options runtime` and `--timestamp`; (c) a notarize step exists containing `notarytool submit` and `--wait`; (d) sign+notarize steps are step-`if`-gated on `secrets.APPLE_CERT_P12`; (e) an unsigned-warning step exists for the secrets-absent path; (f) `.goreleaser.yml` does not exist (guards re-orphaning). | `go test ./scripts/ -run TestReleaseWorkflow` fails on all assertions. | 5 | — |
| T-002 | Split matrix runners: add `runner: ubuntu-latest` (linux) / `runner: macos-latest` (darwin) to the matrix includes; `runs-on: ${{ matrix.runner }}`. | T-001 (a) green; rest still red. | 3 | — |
| T-003 | Add the codesign step to darwin legs: create throwaway keychain, import `APPLE_CERT_P12`/`APPLE_CERT_PASSWORD`, resolve identity via `security find-identity -v -p codesigning` (do NOT hardcode the cert CN), `codesign --sign <identity> --options runtime --timestamp`, then `codesign --verify --strict`. Step-if per decision 2. | T-001 (b)+(d-sign) green. | 5 | — |
| T-004 | Add the notarize step: `ditto -c -k` the signed binary into a zip, `xcrun notarytool submit <zip> --key <(p8) --key-id --issuer --team-id --wait --timeout 20m`, fail the step on `Invalid` status; upload the (signed) bare binary as before. Step-if per decision 2. | T-001 (c)+(d-notarize) green. | 5 | — |
| T-005 | Add the secrets-absent warning step (`if: ${{ secrets.APPLE_CERT_P12 == '' }}`, darwin legs only): `echo "::warning::darwin binary UNSIGNED — Apple secrets not configured (h-jvc1)"`. | T-001 (e) green. | 2 | — |
| T-006 | Delete `.goreleaser.yml`; remove `release`/`release-snapshot` targets from `Makefile` (keep `clean` as-is). | T-001 (f) green; `make -n release` now errors (target gone); `go build ./...` unaffected. | 3 | — |
| T-007 | README: short "Release signing" note — signed+notarized when secrets present, no-staple/online-ticket limitation, pointer to secrets contract in this plan. Full green gate. | `go test ./... && go vet ./... && go build ./...` green; `actionlint .github/workflows/release.yml` clean if actionlint available (skip otherwise, note in PR). | 4 | hatch infra-reviewer (PR review) |

First task is the failing test (TDD, arch §10). The workflow-guard test is hermetic
(pure YAML parse — no Apple calls, no runner execution).

**Validation limit (explicit):** CI-of-CI can only prove workflow STRUCTURE. The
live proof — a tag push producing a signed, notarized, LS-attributable binary — can
only happen after h-jvc1 lands secrets; that live verification is h-jvc1's
acceptance, not this bead's. Do not hold this PR for it.

## Open questions (executor/reviewer-resolvable — no PM items)

- `macos-latest` vs pinned `macos-15`: recommend `macos-latest` (file already uses
  `ubuntu-latest` convention); reviewer may pin if runner drift worries them.
- `notarytool` p8 delivery: recommend writing `APPLE_NOTARY_KEY_P8` to a temp file
  with `mktemp` + cleanup trap rather than process substitution (portability in
  the runner's bash).
- Whether to also `codesign --verify` + `spctl -a -t open --context context:primary-signature`
  post-notarize on the runner: nice-to-have; `notarytool --wait` status Accepted is
  the load-bearing check.

## GDPR data-flow impact

No personal data path. Notarization uploads the compiled binary (zip) to Apple's
notary service — machine code only, no user data embedded. The signature discloses
the org's developer identity (Team ID / org name) — intended and required.
Apple credentials are org secrets stored as GitHub encrypted secrets, never echoed
to logs (keychain import and notarytool read them from env/files; `set -x` must NOT
be enabled in those steps — reviewer checks).

## MDR Class I traceability

No-op. hatch-cli developer tooling, not the voxmemo→voxist-api clinical pipeline.
Heading retained for explicit auditor consideration.

## Status

- [x] T-001 — failing workflow-guard test (7 subtests, all red)     ✅ green at df945db
- [x] T-002 — split matrix runners (macos-latest/ubuntu-latest)     ✅ green at 63a70ab (subtest a)
- [x] T-003 — codesign darwin binary, secret-gated                 ✅ green at 388bd3f (subtest b)
- [x] T-004 — notarize darwin binary via notarytool                ✅ green at 51943ca (subtests c,d)
- [x] T-005 — unsigned-warning step (secrets absent, darwin only)   ✅ green at cf597a4 (subtest e)
- [x] T-006 — delete .goreleaser.yml + Makefile release targets     ✅ green at f1a6a07 (subtest f)
- [x] T-007 — README Release signing note, full gate green          ✅ green at 89e23d8

Full gate: `go test ./...` (662 passed) + `go vet ./...` (clean) + `go build ./...`
(success) all green. `actionlint` not available in this environment — skipped;
flagging for reviewer to run if they have it installed.

Validation limit (per plan, not a gap): live proof of an actual signed+notarized
binary requires h-jvc1 (Apple secrets) to land first — out of scope here.
