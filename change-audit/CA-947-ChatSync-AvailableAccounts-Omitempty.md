# CA-947 — BUG-383: normalize omitted availableAccounts in chat-sync status

## Summary

Runner emits `availableAccounts` with `omitempty`; absent when no Drive
accounts exist. Settings → Projects read `.length` on `undefined` →
"Cannot read properties of undefined" banner. Desktop now normalizes the
payload once at ingest (`normalizeChatSyncGoogleDriveStatus`) and stores the
shape with the field required.

## Verified

- `chatSyncStatus.test.ts`: 2/2 via phase1 node:test pipeline.
- `tsc --noEmit` clean.
