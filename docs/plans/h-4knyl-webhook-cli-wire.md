# Plan — h-4knyl: wire hatch-cli `webhook` to real api.Client + register in root

**Bead:** h-4knyl (P3, hatch-cli)
**Follow-up to:** h-3vggk (PR #3, CLI run bodies green) · molecule h-5fkk9 (hatch-feature-release)
**Branch:** `gc/h-4knyl`

## Problem statement

`hatch-cli`'s `webhook add|list|rm|test` group (`cmd/webhook/webhook.go`) was
built tests-first in PR #3: the cobra commands and run bodies are complete, but
`defaultDeps().NewAPIClient` is hard-`nil` and `webhook.Cmd` is **not registered**
in `cmd/root/root.go`. Both were deliberately deferred because the hatch-api
webhook endpoints (feature h-2o06e) had not landed. **That blocker is now
resolved** — the endpoints are merged to `hatch-api` `main` *and* deployed:

- `hatch-api` `internal/api/router.go:206-210` (on `main`) wires the authed group
  `/v1/apps/{slug}/webhooks`: `GET /` (List), `POST /` (Create),
  `DELETE /{id}` (Delete), `POST /{id}/test` (TestPing).
- Live probe: `GET`/`POST https://api.gethatch.eu/v1/apps/probe/webhooks` → **HTTP 401**
  (route present, auth required), not 404 → deployed.

This plan wires the CLI to those live endpoints in two layers: (1) add the four
webhook methods + a `Webhook` type to `internal/api`, (2) provide a `realAPIClient`
adapter, set `NewAPIClient`, and register the command — exactly mirroring the
existing `cmd/preview` idiom.

## API contract (verified against `hatch-api` `main`)

All routes are Bearer-authed under `/v1/apps/{slug}/webhooks`. The CLI `do()`
helper already prepends `host + "/v1"`, so client method `path` args are
`/apps/{slug}/webhooks…`.

| Method | Route | Request body | Success | Response body |
| --- | --- | --- | --- | --- |
| Create | `POST /apps/{slug}/webhooks` | `{"url":string,"events":[]string}` | 201 (client accepts any 2xx) | `{"id","url","events","secret"}` — `secret` plaintext, **only** on create |
| List | `GET /apps/{slug}/webhooks` | — | 200 | `[{"id","url","events"}]` (no secret, no active/status) |
| Delete | `DELETE /apps/{slug}/webhooks/{id}` | — | 204 No Content | — |
| TestPing | `POST /apps/{slug}/webhooks/{id}/test` | — | 200 | `{"status":"delivered"}` |

Handler source: `hatch-api internal/api/handlers/webhooks.go`
(`webhookCreateRequest{URL,Events}`, `webhookResponse{ID,URL,Events,Secret omitempty}`).

## Existing CLI surface (already built — do NOT rewrite)

`cmd/webhook/webhook.go` already defines the command tree and run bodies and a
**cmd-local** interface + model the run bodies consume:

```go
type Webhook struct { ID, URL string; Events []string; Active bool; LastStatus string }
type APIClient interface {
    CreateWebhook(slug, url string, events []string) (*Webhook, string, error)
    ListWebhooks(slug string) ([]Webhook, error)
    DeleteWebhook(slug, id string) error
    TestWebhook(slug, id string) error
}
```

`defaultDeps().NewAPIClient` is `nil` (guarded loudly in `resolveApp()`).
**Contract gap:** the API `List` returns only `{id,url,events}` — there is no
`active`/`last_status` field server-side. The adapter therefore sets
`Active=true` (webhooks have no disable concept yet) and leaves `LastStatus=""`
(prints `—`). Leaving `Active` at its zero value would make `webhook list` print
`disabled` for every row — a bug. This is captured in T-006 and Open questions.

## Adapter idiom (template: `cmd/preview/preview.go:36-53`)

```go
type realAPIClient struct{ client *api.Client }
func (r *realAPIClient) ListPreviews(p string) ([]api.Preview, error) { return r.client.ListPreviews(p) }
// …
NewAPIClient: func(token string) APIClient { return &realAPIClient{client: api.NewClient(token)} },
```

Webhook differs from preview only in that its interface returns the cmd-local
`webhook.Webhook` (not the raw `api` type), so the adapter **maps fields**.

## Test harness

`internal/api/*_test.go` uses `httptest.NewServer` + `api.NewTestClient(token, srv.URL)`
(see `TestListApps`, `TestGetCronRunLogs`). `cmd/webhook/webhook_test.go` is
`package webhook` (internal), so it can construct the unexported `realAPIClient`
directly and point it at an httptest server via `api.NewTestClient`.

## Micro-tasks

| id | description | acceptance (failing test → green) | est | slings |
| --- | --- | --- | --- | --- |
| T-001 | Write failing test for `(*api.Client).CreateWebhook` in new `internal/api/webhooks_test.go`. | `TestCreateWebhook`: httptest asserts `POST`, path `/v1/apps/demo/webhooks`, `Content-Type: application/json`, body `{"url":"https://h.example/cb","events":["deploy"]}`; server returns 201 `{"id":"wh_1","url":…,"events":["deploy"],"secret":"whsec_abc"}`; asserts returned `*api.Webhook{ID:"wh_1",URL,Events}` **and** `secret=="whsec_abc"`. Compile-fails (no `CreateWebhook`/`api.Webhook`). | 4 | none |
| T-002 | Add `Webhook` type + implement `CreateWebhook`. | Add `type Webhook struct{ ID, URL string; Events []string }` to `internal/api/types.go`; implement `(*Client).CreateWebhook(slug,url string,events []string) (*Webhook,string,error)` in new `internal/api/webhooks.go` (validateSlug → `json.Marshal` body → `do("POST","/apps/"+slug+"/webhooks",…)` → decode `{id,url,events,secret}` into a private response struct → return `&Webhook{…}, secret, nil`). T-001 passes. | 5 | none |
| T-003 | Write failing tests for `ListWebhooks`, `DeleteWebhook`, `TestWebhook`. | Extend `webhooks_test.go`: `TestListWebhooks` (GET → `[{id,url,events}]`, asserts 2 parsed); `TestDeleteWebhook` (asserts `DELETE /v1/apps/demo/webhooks/wh_1`, server 204, nil err); `TestTestWebhook` (asserts `POST …/wh_1/test`, server 200 `{"status":"delivered"}`, nil err). Compile-fails. | 4 | none |
| T-004 | Implement the three methods in `internal/api/webhooks.go`. | `ListWebhooks(slug)` GET+decode `[]Webhook`; `DeleteWebhook(slug,id)` DELETE then `resp.Body.Close()`; `TestWebhook(slug,id)` POST nil body then close. All validate slug. T-003 passes. | 5 | none |
| T-005 | Write failing adapter/wiring test in `cmd/webhook/webhook_test.go`. | `TestRealAPIClient_MapsActive`: build `&realAPIClient{client: api.NewTestClient("t", srv.URL)}`, httptest returns one webhook from List; assert mapped `webhook.Webhook` has `Active==true` & `LastStatus==""`. Plus `TestNewAPIClient_Wired`: `defaultDeps().NewAPIClient != nil`. Compile/assert-fails. | 4 | none |
| T-006 | Add `realAPIClient` adapter + set `NewAPIClient`. | In `cmd/webhook/webhook.go` (or new `cmd/webhook/adapter.go`): `realAPIClient{client *api.Client}` implementing the 4 `APIClient` methods, mapping each `api.Webhook`→`webhook.Webhook{…,Active:true,LastStatus:""}`; `CreateWebhook` threads the secret through; set `defaultDeps().NewAPIClient = func(token string) APIClient { return &realAPIClient{client: api.NewClient(token)} }`. Keep the `resolveApp()` nil-guard (defensive). T-005 passes. | 5 | none |
| T-007 | Write failing test that root registers `webhook`. | In `cmd/root/` test (extend existing `*_test.go`, else add `root_test.go`, `package root`): assert `rootCmd.Commands()` contains a command whose `Name()=="webhook"`. Fails (unregistered). | 3 | none |
| T-008 | Register the command in `cmd/root/root.go`. | Add import `"github.com/EscapeVelocityOperations/hatch-cli/cmd/webhook"` and `rootCmd.AddCommand(webhook.Cmd)` (note: package exposes `var Cmd`, not `NewCmd()` — register the var directly). T-007 passes. | 2 | none |

**Final gate (whole-PR, not a separate bead):** `go build ./... && go vet ./... && go test ./...` all green.

## GDPR data-flow impact

- **New personal data collected/stored by the CLI: none.** The CLI transmits an
  app `slug`, a user-supplied webhook `URL`, and an `events[]` list, and receives
  a signing `secret`. None of these are Voxist-collected data subjects' PII; the
  webhook URL is user-owned infrastructure config.
- **Secret handling:** `CreateWebhook` returns a plaintext signing secret shown
  **once** on stdout (existing `runAdd` body). The CLI must **not** persist it to
  disk or logs. Verified safe: `do()` logs the response **body** only on error
  (`statusCode >= 400`); on a successful create it logs headers only, so the
  secret is never emitted in `--verbose`. T-006 must not add any logging of the
  create response body. `RedactToken` already scrubs the Bearer token in verbose.
- **Data in transit:** TLS to `api.gethatch.eu`; Bearer auth via existing
  `auth.GetToken`. No change to the auth/token storage path.
- **Right to erasure / retention:** webhook records live server-side
  (`hatch-api`, encrypted secret at rest); deletion is the existing
  `DELETE …/{id}` path now reachable from the CLI. No CLI-side retention.

## MDR Class I traceability

Not applicable. `hatch-cli` is PaaS developer tooling for gethatch.eu; it is
outside the voxmemo → voxist-api clinical chain-of-evidence. No
microphone-to-clinical-note traceability metadata is touched. Heading retained
for auditor visibility.

## Open questions (executor / reviewer-resolvable — no PM gate)

1. **Registration idiom.** `cmd/webhook` exposes `var Cmd`; every other command
   exposes `NewCmd()`. Plan registers `webhook.Cmd` directly (minimal diff). If
   the reviewer prefers convention parity, add `func NewCmd() *cobra.Command { return Cmd }`
   and register `webhook.NewCmd()`. Cosmetic; either passes T-007.
2. **`api.Webhook` field set.** API never returns `active`/`last_status`. Adapter
   hardcodes `Active=true`. If/when hatch-api adds delivery-status tracking
   (separate feature), revisit the mapping and surface real status in
   `webhook list`. File a follow-up then; out of scope here.
3. **Method-file placement.** Plan adds `internal/api/webhooks.go` (mirrors the
   resource-specific `internal/api/packs.go`). Reviewer may prefer folding into
   `client.go` alongside `AddDomain`/`RemoveDomain`; either is idiomatic.

## Handoff

Single PR off `gc/h-4knyl` against `main`. Self-operated rig: per molecule
h-5fkk9, deploy/release of the CLI binary is owned by the release lane, not this
bead — this bead's DoD is the merged PR (CLI built, vetted, tested green).
Manual end-to-end smoke (`hatch webhook add/list/test/rm` against
api.gethatch.eu) is possible since the API is live, but is **not** required for
the PR — the acceptance tests use httptest and fully cover the contract.
