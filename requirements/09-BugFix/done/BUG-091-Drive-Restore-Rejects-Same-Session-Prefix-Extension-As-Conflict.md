# Metadata

- Document ID: `BUG-091`
- Title: `Drive Restore Rejects Same-Session Prefix Extension As Conflict`
- Phase: `bugfix`
- Status: `done`
- Owner: `DatNguyen`
- Reviewers: `—`
- Created: `2026-06-19`
- Last Updated: `2026-06-19`
- Parent Documents: [SD-14: Codex Cross-Account Chat Resume And Home Sync](../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- Child Documents: `—`
- Related Documents: [CA-106-fix-drive-restore-prefix-extension-conflict](../../../change-audit/CA-106-fix-drive-restore-prefix-extension-conflict.md)
- Replaces: `—`
- Tags: `codex, google-drive, chat-session-sync, restore, conflict, cross-pc`

## AI Quick View

### Summary

- `restoreChatRunFromDrive` rejects a restore with `session_file_conflict` whenever the local rollout file hash differs from the remote file hash, even when the two files are prefix-compatible extensions of the same Codex session.
- A Codex rollout file grows by appending JSONL lines, so a mismatch is not always divergence — it can mean one side has more turns than the other.
- The local relocation path already handles this correctly via `updateCodexDestinationIfSameSessionExtends` (`bytes.HasPrefix` check), but the Drive restore path did not apply the same logic.
- Two legitimate cases were incorrectly blocked: remote is newer than local (should overwrite local), and local is ahead of remote (should keep local unchanged).
- Prefix-extension acceptance is gated to Codex only; other providers keep strict hash equality because their session-file append semantics are not confirmed.
- When local is ahead, the restore must keep the newer local conversation metadata rather than downgrading it to the older remote manifest.

### Current Ask

- Apply the same prefix-extension semantics to the Drive restore conflict check that the local relocation path already uses.

### Key Decisions

- `D-1` Mirror the `bytes.HasPrefix` logic from `updateCodexDestinationIfSameSessionExtends` into the restore path so prefix-compatible extension is accepted, not rejected.
- `D-2` Only return `session_file_conflict` when the bytes are genuinely divergent (neither side is a prefix of the other).
- `D-3` Gate prefix-extension acceptance to `ProviderKeyCodex`. The restore branch is generic and also serves Claude (`restoreTargetPath`/`RestoreSessionFile`), but Claude session-file append semantics are not confirmed, so non-Codex providers keep strict hash equality.
- `D-4` In the local-ahead case, preserve the existing local session metadata (`LastPrompt`, `LastMessage`, `Status`, `UpdatedAt`) instead of overwriting it with the older remote manifest.

### Constraints

- The fix must not weaken protection against genuinely divergent files — two different session chains that happen to share the same id string must still be rejected.
- The restore branch (`restoreChatRunFromDrive`) is provider-generic; the prefix-extension behavior is intentionally scoped to Codex only.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/chat_session_sync.go:628` — fixed conflict check
- `apps/local-runner/internal/runner/session_file_locator.go:119` — `updateCodexDestinationIfSameSessionExtends` (reference logic)
- [SD-14 §Q-3b Known Gap](../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)

## 1. Issue Summary

When PC B tries to restore a Drive-synced Codex chat and it already holds a local copy of the same rollout file (from a prior restore), the restore fails with `session_file_conflict` if the local and remote bytes differ at all — even if one is simply a later extension of the other.

Codex rollout files are JSONL files that grow by appending new lines as the chat continues. So after PC A continues the chat and re-syncs, the Drive file has more content than the old local copy on PC B. The hash differs. The restore correctly rejects genuinely divergent files, but it was incorrectly rejecting prefix-compatible extensions (one side is a byte-prefix of the other), which are the same session at a different point in time.

## 2. Parent Links

- impacted tech design: [SD-14: Codex Cross-Account Chat Resume And Home Sync](../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md) §Q-3b

## 3. Environment and Reproduction

- environment: any cross-PC Drive sync scenario where the chat was continued after a prior restore
- reproduction steps:
  1. PC A starts a Codex chat; syncs to Drive (3-turn rollout file).
  2. PC B restores the chat from Drive (3-turn file written locally).
  3. PC A continues the chat (now 5 turns), re-syncs to Drive.
  4. PC B attempts to restore again — receives `session_file_conflict`.
- frequency: reproducible on any re-restore after the source chat has had more turns

## 4. Expected vs Actual

- expected: restore accepts the 5-turn remote file, overwrites the local 3-turn file (remote extends local)
- actual: `session_file_conflict` error returned immediately; no write performed

Second case:

- expected: if local already has 5 turns and Drive has 3 turns, restore skips the write and keeps local unchanged
- actual: `session_file_conflict` error returned

## 5. Impact

- users affected: any cross-PC Codex chat user who restores a chat more than once as the chat grows
- workflows affected: Drive-backed Codex chat restore (`restoreChatRunFromDrive`)
- severity: medium — makes cross-PC Codex re-restore fail silently as a conflict instead of succeeding

## 6. Root Cause

- hypothesis: the restore conflict check was a simple hash equality, not a prefix-extension check
- confirmed cause: `chat_session_sync.go:629` — `hashBytesSHA256(existing) != manifest.ProviderFile.SHA256` was the sole condition for `session_file_conflict`; no prefix check was applied
- evidence: code inspection confirms the `bytes.HasPrefix` logic exists in the local relocation path (`session_file_locator.go:139,145`) but was absent from the restore path

## 7. Fix Strategy

- `F-1` Add `"bytes"` to the import block in `chat_session_sync.go`.
- `F-2` Replace the single hash-equality conflict guard with a four-branch check, with prefix-extension gated to Codex:
  - if `hashBytesSHA256(existing) == manifest.ProviderFile.SHA256` → identical, no write
  - else if Codex and `bytes.HasPrefix(providerBytes, existing)` → remote extends local, overwrite local
  - else if Codex and `bytes.HasPrefix(existing, providerBytes)` → local already extends remote, keep local (set `localAhead`)
  - else → genuinely divergent (or non-Codex mismatch), return `session_file_conflict`
- `F-3` When `localAhead` is set, before persisting the session row, read the existing local session via `SessionHistoryReader.GetProviderSession` and preserve its non-empty `LastPrompt`, `LastMessage`, `Status`, and `UpdatedAt` so the older remote manifest does not downgrade local history.

### Decision flow (what the code does at runtime)

```
Does a local file exist at targetPath?
│
├─ NO (ErrNotExist)
│   → write from Drive via RestoreSessionFile   (normal first restore)
│
├─ YES, read OK
│     codexExtend = (manifest.ProviderKey == ProviderKeyCodex)
│
│     ┌─ hash(local) == remote SHA256
│     │   → identical — skip write entirely
│     │
│     ├─ codexExtend && bytes.HasPrefix(remote, local)
│     │   → remote has MORE turns than local
│     │   → overwrite local with newer remote bytes
│     │
│     ├─ codexExtend && bytes.HasPrefix(local, remote)
│     │   → local has MORE turns than remote
│     │   → keep local file untouched; set localAhead = true
│     │
│     └─ anything else
│         → session_file_conflict  ← only true conflict
│
└─ YES, read failed (not ErrNotExist)
    → workflow_state_unavailable
```

**Why `bytes.HasPrefix` works:** Codex rollout files are JSONL — each turn appends a new line. If the remote file is ahead, it starts with every byte the local file has and then has extra lines after. `HasPrefix(remote, local)` captures exactly that. The reverse (`HasPrefix(local, remote)`) means the local machine continued the chat after the Drive snapshot was taken.

**What `codexExtend` is:** a local boolean (`codexExtend := manifest.ProviderKey == ProviderKeyCodex`) that gates the two `HasPrefix` branches. Its only job is to ensure prefix-extension logic activates for Codex only — nothing more.

**Why Claude was unaffected by the bug:** The old strict hash-equality check applied to all providers. In theory a Claude session hitting a hash mismatch would also fail with `session_file_conflict`. In practice Claude never reaches that condition: Claude does not use an append-only rollout file that grows turn-by-turn the way Codex does, so the "local file exists with different bytes from remote" scenario does not arise during Claude restores. The fix does not change Claude's path — non-Codex sessions still fall through to `default` → `session_file_conflict` on any mismatch, same as before.

## 8. Validation

- `V-1` Full sync/restore/relocate suite passes: `go test ./internal/runner/... -run "TestRestore|TestChatSession|TestSync|TestRelocate|TestResolveRestored"` — 48 tests pass; `go vet ./internal/runner/` clean.
- `V-2` New direct Drive-restore tests added in `chat_session_sync_test.go`:
  - `TestRestoreChatRunFromDriveOverwritesWhenRemoteExtendsLocal` — remote-extends-local overwrites local file
  - `TestRestoreChatRunFromDriveKeepsLocalWhenLocalExtendsRemote` — local-ahead keeps local file untouched
  - `TestRestoreChatRunFromDrivePreservesLocalMetadataWhenLocalAhead` — local-ahead keeps newer local metadata
- `V-3` Existing `TestRestoreChatRunFromDriveRejectsOverwriteConflict` still returns `session_file_conflict` for genuinely divergent content.

## 9. Regression Guard

- tests: three new direct restore tests cover both prefix-extension directions and the metadata-preservation path; the existing conflict and identical-file tests guard the divergent and equal paths
- alerts: `session_file_conflict` errors in Drive restore logs should decrease after this fix
- audit checks: verify that a re-restore of a continued chat no longer returns `session_file_conflict` and does not downgrade history metadata

## 10. Follow-Up Document Updates

- upstream docs that must change: SD-14 §Q-3b updated in this session to document the gap and the fix — now resolved
- notes left unchanged on purpose: the design rule "never silently merge genuinely divergent rollout states" is preserved; only the definition of "conflict" is narrowed to exclude same-session prefix-compatible extensions, and only for Codex
