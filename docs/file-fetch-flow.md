# File Fetch Flow (Bridge + gittr UI)

This page documents how the git-nostr-bridge fits into gittr’s file fetching pipeline so other
clients can reproduce the same behavior.

> 🆕 indicates fork-only pieces (HTTP fast lane, dedupe channel, watch-all mode) that are not yet upstream.

## 1. What the bridge already exposes

- **Repository mirror**: when a NIP-34 event hits the relays (or `/api/event` via `BRIDGE_HTTP_PORT`),
  the bridge clones/updates the bare repo under `repositoryDir`.
- **File tree API**: once cloned, a GET on `http://<bridge>/api/nostr/repo/tree?repo=<pk>/<name>` returns
  a flat file list (used for directory views).
- **File content API**: GET `.../api/nostr/repo/file-content?repo=<pk>/<name>&path=<file>&branch=<ref>`
  streams blob contents.
- **Clone trigger**: if the repo is missing, gittr asks `.../api/nostr/repo/clone` and the bridge pulls
  it from the `clone`/`source` tags in the NIP-34 event, then broadcasts a `grasp-repo-cloned` SSE for
  auto-refresh.
- **Blossom clones**: any HTTPS clone URL (including `https://blossom...`) is treated like a normal
  Git remote; the bridge just runs `git clone` against it (no extra APIs).
- **Blossom URLs**: any HTTPS clone URL (including `https://blossom...`) is treated like a normal
  Git remote; the bridge doesn’t need Blossom-specific APIs.

## 2. UI flow recap (gittr)

1. **User opens a repo tab** (files, issues, PRs, commits, etc.).
2. UI tries cached data → embedded NIP-34 files → bridge tree API.
3. 🆕 If the bridge returns 404, gittr triggers `repo/clone`, waits ~3 seconds, retries tree API (and consumes the `grasp-repo-cloned` SSE).
4. If still missing, UI falls back to GitHub/GitLab/Codeberg APIs using the normalized `source` URLs.
   - **GitLab pagination**: GitLab API returns max 100 items per page - gittr implements pagination to fetch ALL files (critical for repos with >100 files)
5. File open actions follow the same order: cache → embedded content → 🆕 multi-source fetch (bridge + external) → Nostr fallback → git servers.

This is described in detail in gittr's [`docs/FILE_FETCHING_INSIGHTS.md`](https://github.com/arbadacarbaYK/gittr/blob/main/docs/FILE_FETCHING_INSIGHTS.md), but the bridge only needs to
provide step 2/3 above.

### Push to Nostr Process

When pushing a repository to Nostr, the file content source follows this order:

1. **localStorage** (primary) - Files should already be present from create/import workflow
2. **Bridge API** (fallback) - If files are missing from `localStorage`, fetch from `/api/nostr/repo/file-content`
3. **Exclusion** - Files without content are excluded with warnings

**Important**: The push process does NOT fetch files from external sources (GitHub, GitLab, etc.) during push. Files must already be available in `localStorage` or on the bridge. If files are missing, users should re-import the repository.

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
  API to know when the repo is ready.

With these pieces, any frontend can implement the same file list/content fallbacks shown in gittr’s
docs, while the bridge remains host-agnostic.

