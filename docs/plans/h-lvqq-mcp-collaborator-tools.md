# Plan — h-lvqq: MCP collaborator/invite tools (ADR-0022 P3)

**Bead:** h-lvqq · **Rig:** hatch/hatch-cli · **Spec:** `/Users/cstar/portharbour/docs/ADR-0022-egg-sharing-multi-user.md`

## Stacked on gc/h-eipa (P2), not main

The MCP server (`internal/mcpserver/server.go`) lives in the SAME hatch-cli
repo as the CLI commands (`cmd/collab`, `cmd/invite`) built for h-eipa — this
bead needs the exact same `internal/api/collaborators.go` Client methods
h-eipa already added, unmerged on `origin/main` as of this bead's start.
Branched from `gc/h-eipa` (PR #17) instead of `main` to reuse them rather
than duplicate. `gc.base_branch` metadata set accordingly. **This PR will
show h-eipa's commits until #17 merges — rebase onto main after.**

The MCP plugin Claude Code has installed (`~/.claude/plugins/marketplaces/hatch`)
is confirmed (via `.claude-plugin/plugin.json`'s `mcpServers.hatch.command:
"hatch", args: ["mcp"]`) to be a shallow clone of this exact repo — there is
no separate plugin source to edit.

## Tool scope (exactly 5, per the bead — no decline_invite)

`share_app`, `list_collaborators`, `unshare_app`, `list_pending_invites`,
`accept_invite`. The ADR's own phasing list omits decline; honoring that
boundary exactly rather than adding a 6th tool unasked.

## Micro-tasks (each: RED test using the existing `newMockServer`/`saveAndRestore` harness, then GREEN tool+handler, mirroring `add_domain`/`list_domains`/`remove_domain`)

- [x] T-001..T-005 — RED (20 tests appended to `server_test.go`, confirmed fails to compile: 10 `undefined` errors before any handler existed) → GREEN (all 5 tool+handler pairs in `server.go`, mirroring `add_domain`/`list_domains`/`remove_domain` exactly): `share_app`, `list_collaborators`, `unshare_app` (email→ID resolution mirrors CLI `collab rm` — no server remove-by-email endpoint), `list_pending_invites` (no `app` param), `accept_invite`.   ✅ 20/20 green (654 suite: no regressions)
- [x] T-006 — Registered all 5 in `NewServer()` under a new "Collaboration operations (ADR-0022)" category.
- [x] T-007 — Full suite gate (`go build`, `go vet`, `gofmt -l` clean, `golangci-lint` — the only 12 findings are pre-existing, all at line numbers well before this bead's additions) + a REAL MCP protocol round-trip: built the actual binary, spawned `hatch mcp`, sent real `initialize`+`tools/list` JSON-RPC over stdio, confirmed all 5 new tools appear with correct schemas/descriptions in the live response (not just unit-tested).

## Full suite gate before PR

`cd hatch-cli && go build ./... && go test ./... -race`
