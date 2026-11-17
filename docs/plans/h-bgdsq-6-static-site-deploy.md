# Static sites: `hatch deploy --runtime static` sends changed files only (T-506, h-bgdsq.6)

**Status:** implemented · **Repo:** hatch-cli only (CLI and MCP `deploy_app`)
**Spec:** hatch-api `docs/plans/h-qa0yv-sites-statiques-stockage-objet.md` §10 T-506
**Client contract:** hatch-api `docs/plans/h-bgdsq-9-site-ingest-relay.md` (T-505b relay, manifest → blobs → confirmation),
`POST /v1/apps/{slug}/site` (T-501), preview `kind` (T-505, `docs/plans/h-bgdsq-5-site-history-quotas-previews.md` §4)

## What changes

`hatch deploy --runtime static` and the MCP tool `deploy_app` with `runtime: "static"`, on an egg that **already serves a
Hatch site**, no longer build an artifact. They:

1. list the deploy target the way the artifact is built (`.hatchignore` or the built-in defaults; `.git/`, `.DS_Store`,
   `.hatch.toml` and env files never go; a `.hatchignore` that re-includes an env file is refused) and hash each file
   (path in NFC with `/`, sha256, size); a symlink to a file inside the target is published with that file's content,
   any other symlink or special file is skipped and listed;
2. post the list to `POST /v1/apps/{slug}/site/ingest` (JSON) and get the sha256 the platform lacks;
3. stream a gzipped tar of exactly those blobs (entry name = sha256) to the same route, chunked, never held in memory;
4. post the list again until `missing == []` (after a cut or a `502`, only what is still missing goes, at most 3 uploads);
5. publish the list with `POST /v1/apps/{slug}/site`: the version is live when the API answers `201`.

Nothing waits for a rollout: there is none. `--json` prints `status: "live"`, `waited: false`, `kind: "site"`,
`site_version`, `deployment_id`, `files`, `uploaded_files`, `uploaded_bytes`, `sent_bytes`.

Every other deploy is the artifact deploy, byte for byte as before: other runtimes (never asked), a new egg, an egg with no
site, a platform with sites off.

## Signal: is the egg a site?

hatch-api exposes no "migrated" flag. Two signals exist and are used:

* **An egg:** `GET /v1/apps/{slug}/deployments` holds a row with `"kind":"site"`. hatch-api writes one per site publication
  and rollback (T-505), in the same transaction as the live pointer, and never deletes them; image rows carry no `kind`, and
  an API older than T-505 has no such row. This route is not behind `HATCH_SITES_ENABLED`, so the switch is read next:
* **Switch off:** the first manifest post answers `404 sites_disabled` → the artifact deploy, unchanged.
* **A preview** (`--preview`): the preview answer's `kind` (`"site"` when the parent serves a live site, T-505 §4). Absent or
  `"image"` → artifact. For static previews the artifact is built after the preview is created (its kind decides).

The check runs for `--runtime static` with a known egg only (`--app`, or `.hatch.toml`; the MCP tool: `app`, or the
`.hatch.toml` of `deploy_target`), before any artifact is built: a site may be larger than an artifact can be (1 GiB).
A `404` from the history tells nothing (artifact path, which reports an unknown egg itself). Any other failure to read it
stops the deploy (exit 1): building an image for an egg that may be a site is the wrong answer.

Consequence for the migration (§11 of the study): an egg starts taking this path once its first site version exists
(published by the operator). If an egg is routed back to its image after that (§11 step 5), its history still has site rows,
so `hatch deploy` keeps publishing sites to it: the rollback of a migration has to account for it.

## Refusals and exit status

A refusal is printed with its status, its code and what it means, e.g.
`sending the file list: API error 413: site_quota_exceeded: the egg's site storage quota would be exceeded (all versions count): reduce the site`.
All site failures exit **1** (`ExitDeployError`), outside a TTY too (h-59ej), and `--json` prints
`{"status":"error","waited":false,"error":"…","slug":"…"}`. Retried: a broken connection (not a timeout), `502
site_ingest_failed`, `400 site_upload_interrupted` (the list is posted again, only what is missing goes). Not retried: `503
sites_unavailable` (an operator problem) and every 4xx. The confirmation is retried only when the connection was never
opened: after a timeout or a 5xx the message says that deploying again is safe (it uploads nothing, publishes one version more).

## Measure: 5 % of the files changed

`internal/sitedeploy` `TestPublish_FivePercentChangeSendsAboutFivePercent`: 400 files of 32 to 96 KiB of random
(incompressible) content, 24.7 MiB; every 20th file rewritten (20 files, 5.06 % of the bytes); redeploy:

| | bytes | share of the site |
|---|---|---|
| blobs (gzipped tar, on the wire) | 1 328 256 | **5.13 %** |
| file lists (two manifest posts and the confirmation) | 150 033 | 0.58 % |
| **total** | 1 478 289 | **5.71 %** |

An artifact redeploy sends the whole site (≈ 100 % for incompressible content). The file lists cost about 125 bytes per file
per post (three posts per deploy): 0.6 % here, about 0.2 % on the study's 1 GiB / 6 332-file bench (M7: 5.5 %).

## Old CLI

The old CLI (`d9e0cb3`, 2026-11-11) deploys an egg without a site as before: it posts `POST /v1/apps/{slug}/artifact`
(`application/gzip`, `X-Artifact-Metadata`), and in hatch-api main (`72927c3`) that route, its handler (`artifact.go`, last
changed by `b72beb9`, 2026-11-12, the 1 GiB limit) and its body bound and timeout in `router.go` are unchanged by T-501 to
T-505b, which only add `/site` routes. Checked by reading; nothing to change in the API.

## Code

* `internal/api/sites.go`: `HasSite`, `IngestSiteManifest`, `UploadSiteBlobs`, `PublishSite`, `SiteError` (status + code),
  `IsSitesDisabled`; `Preview.Kind`.
* `internal/sitedeploy`: `Build` (the list), `Publish` (the contract, retries); `sitetest`: a simulated hatch-api for tests.
* `cmd/deploy/local.go`: `deploySiteIfMigrated`, `deploySite`; `internal/mcpserver/deploy_site.go`: `deploySiteApp`.

## Not in this change

API, control, hatch-admin, release, deployment. No way to force the site path on an egg without a site (the first
publication of a migration is the operator's). No pruning of versions.

## Rollback

Revert the PR: every static deploy goes back to the artifact path.
