# BUG-383: Settings Projects panel crashes on missing `availableAccounts`

- Document ID: `BUG-383`
- Status: `done`
- Severity: medium — Settings banner "Cannot read properties of undefined (reading 'length')", chat-sync section unusable
- Area: `apps/desktop-flowpilot/src/components/settings/ProjectsSettings.tsx`

## Symptom

Settings → Projects shows the banner `Cannot read properties of undefined
(reading 'length')` on load for any project without connected Google Drive
accounts.

## Root cause

`ChatSyncGoogleDriveConnectionStatus.availableAccounts` is `omitempty` in the
runner (`types.go`), so the status response omits the key entirely when no
Drive accounts exist. `loadChatSyncStatus` then read
`payload.availableAccounts.length` on `undefined` → TypeError surfaced as the
raw banner text. Six JSX consumers had the same latent crash.

## Fix

- `availableAccounts` typed optional (matches the wire).
- New `normalizeChatSyncGoogleDriveStatus` fills omitted fields once at ingest;
  `chatSyncStatus` state carries the normalized (required) shape so all
  downstream `.length`/`.map` sites are safe.

## Verified

`chatSyncStatus.test.ts` (node:test, phase1 pipeline): missing key → `[]`,
present array preserved — 2/2 pass. `tsc --noEmit` clean.
