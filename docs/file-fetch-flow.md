# File Fetch Flow (Bridge + gittr UI)

This page documents how the git-nostr-bridge fits into gittr’s file fetching pipeline so other
clients can reproduce the same behavior.

> 🆕 indicates fork-only pieces (HTTP fast lane, dedupe channel, watch-all mode) that are not yet upstream.

## 1. What the bridge already exposes

- **Repository mirror**: when a NIP-34 event hits the relays (or `/api/event` via `BRIDGE_HTTP_PORT`),
  the bridge clones/updates the bare repo under `repositoryDir`.
- **File tree API**: once cloned, a GET on `http://<bridge>/api/nostr/repo/tree?repo=<pk>/<name>` returns
  a flat file list (used for directory views). On **gittr.space**, the browser typically calls the **Next.js** route **`GET https://gittr.space/api/nostr/repo/files`** (same on-disk `repositoryDir/{pubkey}/{repo}.git` as the bridge), not only the bridge’s internal HTTP origin.
- **File content API**: GET `.../api/nostr/repo/file-content?repo=<pk>/<name>&path=<file>&branch=<ref>`
  streams blob contents.
  - **CRITICAL**: File paths in the `path` parameter must be URL-encoded using `encodeURIComponent()` to handle non-ASCII characters (Cyrillic, Chinese, accented characters, etc.). The API automatically decodes them and handles UTF-8 correctly.
  - Example: `path=${encodeURIComponent('ЧИТАЙ.md')}` or `path=${encodeURIComponent('读我D.md')}`
- **Clone trigger**: if the repo is missing on disk, gittr can `POST .../api/nostr/repo/clone` (bare mirror from a `clone` HTTPS URL), then read the tree from `/api/nostr/repo/files`. The UI may also call **`GET /api/git/repo-files?sourceUrl=...`** to shallow-clone a remote GRASP URL into a temp directory and show files **before** the bare mirror finishes.
- **Blossom clones**: any HTTPS clone URL (including `https://blossom...`) is treated like a normal
  Git remote; the bridge just runs `git clone` against it (no extra APIs).

## 2. UI flow recap (gittr)

1. **User opens a repo tab** (files, issues, PRs, commits, etc.).
2. UI tries cached data → embedded NIP-34 files → bridge tree API.
3. 🆕 For each **GRASP HTTPS** URL in the latest kind **30617** `clone[]` tags (in parallel):
   - **`GET /api/nostr/repo/files`** (on-disk mirror)
   - If empty/404: **`GET /api/git/repo-files?sourceUrl=<that URL>`** (remote shallow clone — first working mirror wins the race)
   - If still needed: **`POST /api/nostr/repo/clone`**, await/poll bridge, optional `grasp-repo-cloned` event
4. GitHub / GitLab / Codeberg are tried early when `source` or GitHub clone URLs exist (`prioritizeUpstreamCloneUrls`).
   - **GitLab pagination**: GitLab API returns max 100 items per page - gittr implements pagination to fetch ALL files (critical for repos with >100 files)
5. File open actions follow the same order: cache → embedded content → multi-source fetch (bridge + upstream + GRASP shallow) → Nostr fallback.

**Newest metadata:** latest kind **30617** on relays.  
**Newest commit across every GRASP mirror:** not fully compared in gittr today — first successful tree in the parallel race, not max `HEAD` across servers. See gittr [`docs/FILE_FETCHING_INSIGHTS.md`](https://github.com/arbadacarbaYK/gittr/blob/main/docs/FILE_FETCHING_INSIGHTS.md).

This is described in detail in gittr's [`docs/FILE_FETCHING_INSIGHTS.md`](https://github.com/arbadacarbaYK/gittr/blob/main/docs/FILE_FETCHING_INSIGHTS.md), but the bridge only needs to
provide the on-disk mirror (steps 1–3 in section 1 above).

### Push to Nostr Process

When pushing a repository to Nostr, the file content source follows this order:

1. **localStorage** (primary) - Files should already be present from create/import workflow
2. **Bridge API** (fallback) - If files are missing from `localStorage`, fetch from `/api/nostr/repo/file-content`
3. **Exclusion** - Files without content are excluded with warnings

**Important**: The push process does NOT fetch files from external sources (GitHub, GitLab, etc.) during push. Files must already be available in `localStorage` or on the bridge. If files are missing, users should re-import the repository.

**Empty Commit File Preservation**: When pushing with no file changes (e.g., to update commit date after refetch), the push API (`/api/nostr/repo/push`) automatically preserves existing files from the repo. This ensures that date-update commits maintain the file tree, allowing other clients to display files correctly. The commit is still created with `--allow-empty` to ensure a new commit is always created with the current timestamp.

## 3. What’s “new” in this fork

- **HTTP fast lane** (`BRIDGE_HTTP_PORT`): lets the UI POST a signed NIP-34 event straight to the
  bridge so the repo is mirrored immediately instead of waiting for relay propagation.
- **Deduplication channel** (`mergedEvents` + `seenEventIDs` cache): merges HTTP and relay events into a single stream, then uses a seen-event cache to ensure the same event doesn't clone twice.
- **Watch-all mode**: leaving `gitRepoOwners` empty mirrors *every* repo, which is how gittr builds
  the public “Browse” list.

## 4. How other clients can reuse it

- Publish regular gitnostr events (kinds 50, 51, 30617) and the bridge will mirror them exactly as
  gittr does.
- Use the tree and file-content endpoints for any UI (web, CLI) that needs file browsing without
  cloning locally.
- If you want instant confirmation after publishing, enable the HTTP API via `BRIDGE_HTTP_PORT` and
  POST the same event JSON you sent to relays.
- For GRASP-compatible flows, listen for the `grasp-repo-cloned` event (SSE) after calling the clone
  API to know when the on-disk mirror is ready (optional if you already use remote shallow clone like gittr’s `/api/git/repo-files`).

With these pieces, any frontend can implement the same file list/content fallbacks shown in gittr’s
docs, while the bridge remains host-agnostic.
