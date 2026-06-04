## Problem

When an artifact exists only in remote storage and there is no matching local snapshot on the current machine, the item title shown in the `/artifacts` page under `Artifact generated list` falls back to the remote record title.

For many workflow output artifacts that fallback title is the generic file name `Response.md`, so the card title is not meaningful.

## Reproduction

1. Generate and sync artifacts on machine A.
2. Open the same project on machine B without a local artifact snapshot.
3. Visit `/artifacts`.
4. Open `Artifact generated list` and switch to `Remote/Sync`.

Current result:
Remote-only items often display `Response.md`.

Expected result:
Remote-only items should display the actual artifact title summary, not the generic file name.

## Root Cause

The `/artifacts` browser panel currently derives the title from local `contentMarkdown` only.

- If a local snapshot exists, the panel truncates the content into a readable summary.
- If no local snapshot exists, the panel falls back directly to the remote row title.
- For synced workflow outputs, that remote row title is frequently the generic canonical filename `Response.md`.

This is a UI-side issue in `apps/admin-web`, not a Go-runner sync issue.

## Working Reference

`/workflow-runs` already handles this case correctly.

Its title loader treats generic remote titles like `Response.md` as placeholders and loads the remote artifact content to derive a short summary instead.

Reference implementation:

- `apps/admin-web/src/lib/workflow-run-title.ts`
- `apps/admin-web/src/routes/_authenticated/workflow-runs.tsx`

## Fix Direction

Update the `/artifacts` page and the shared `ArtifactRunBrowserPanel` so that remote-only artifacts follow the same fallback strategy:

1. Keep using local `contentMarkdown` when a local snapshot exists.
2. If the artifact is remote-only and the title is generic (`Response.md` or `artifact.md`), load the remote content through the existing artifact open endpoint.
3. Derive a short summary from that remote content and use it as the card title.
4. Keep the stored remote title when it is already meaningful and non-generic.

## Acceptance Criteria

1. A remote-only synced artifact no longer shows `Response.md` as the card title when its remote content is available.
2. The title is derived from the exact remote artifact content and truncated consistently with the existing workflow-runs behavior.
3. Existing local artifact title behavior remains unchanged.
4. Existing open-link behavior for remote artifacts remains unchanged.
