# Plan — h-eipa: CLI collaborator/invite commands (ADR-0022 P2)

**Bead:** h-eipa · **Rig:** hatch/hatch-cli · **Spec:** `/Users/cstar/portharbour/docs/ADR-0022-egg-sharing-multi-user.md`
Self-sequenced from the bead's own spec (architect-authored ADR; disjoint from P1, wraps the existing hatch-api endpoints 1:1 — no new server work).

## Cross-rig finding (flagged elsewhere, not acted on here)

hatch-landing has its OWN separate Prisma-backed collaborator implementation,
not a proxy to hatch-api (confirmed by reading its server routes). Flagged on
h-nzeh (P4) and h-qxvp (umbrella) via bd comment. Does not affect this bead —
the CLI talks to hatch-api directly, per the bead's own "wraps existing API"
framing.

## Wire-format facts (checked against the live hatch-api handlers, not assumed)

- Server endpoints (`internal/api/handlers/collaborators.go` in hatch-api):
  `POST/GET /v1/apps/{slug}/collaborators`, `DELETE .../collaborators/{id}`,
  `GET /v1/invitations/pending`, `POST /v1/invitations/{token}/accept|decline`.
- Invite/Accept responses are the raw `db.AppCollaborator` row. `ListCollaborators`
  returns `db.ListCollaboratorsByAppRow` (adds `user_email`). `ListPendingInvites`
  returns `db.ListPendingInvitesForEmailRow` (adds `app_slug`/`app_name`/`invited_by_email`
  — explicit JSON tags, clean types, good for CLI display).
- `pgtype.UUID`/`pgtype.Text` marshal cleanly (plain string or `null`) — verified
  against the pgx v5 source (`MarshalJSON`), not assumed.
- `AcceptedAt` is `sql.NullTime` (stdlib, no custom `MarshalJSON`) → marshals as a
  nested `{"Time":...,"Valid":...}` object, not a plain date. Deliberately NOT
  decoded client-side (unnecessary for CLI display — `Status` already conveys
  accepted/pending/declined); out of scope to fix the server response shape here.
- No server endpoint removes-by-email — only by collaborator UUID. `collab rm`
  accepting `[email|id]` per the bead means the CLI resolves email→ID itself via
  `ListCollaborators` before calling `RemoveCollaborator`.

## Micro-tasks

- [x] T-001/T-002/T-003 — `internal/api/collaborators.go` (types + 6 `*Client` methods) + `collaborators_test.go` (RED confirmed: 7 compile errors before implementation).   ✅ green at TestInviteCollaborator(+MaxReachedError)/TestListCollaborators/TestRemoveCollaborator/TestListPendingInvites/TestAcceptInvite/TestDeclineInvite (607 suite: no regressions). Note: wrote the full implementation once before its test by mistake, caught it, deleted it, and redid RED→GREEN properly.
- [ ] T-004 — RED: `cmd/collab/collab_test.go` — table-driven tests for `add`/`ls`/`rm` via injectable `Deps` (mirrors `cmd/domain`). Fails to compile.
- [ ] T-005 — GREEN: `cmd/collab/collab.go` — `hatch collab add|ls|rm <slug> [email|id]`. `add`'s success output states the secrets-trust implication (ADR-0022 §"Sharing an egg shares its secrets" — must be surfaced in CLI copy). `rm` resolves email→ID client-side when given an email.
- [ ] T-006 — RED: `cmd/invite/invite_test.go` — table-driven tests for `ls`/`accept`/`decline`. Fails to compile.
- [ ] T-007 — GREEN: `cmd/invite/invite.go` — `hatch invite ls|accept|decline [token]`.
- [ ] T-008 — Wire `collab.NewCmd()`/`invite.NewCmd()` into `cmd/root/root.go`.
- [ ] T-009 — Full suite gate (`go build ./...`, `go vet ./...`, `gofmt -l`, `golangci-lint`, `go test ./... -race`), PR, route to reviewer.

## Full suite gate before PR

`cd hatch-cli && go build ./... && go test ./... -race`
