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
- Related Documents: [CA-091-fix-drive-restore-prefix-extension-conflict](../../../change-audit/CA-091-fix-drive-restore-prefix-extension-conflict.md)
- Replaces: `—`
- Tags: `codex, google-drive, chat-session-sync, restore, conflict, cross-pc`

## AI Quick View

### Summary

- `restoreChatRunFromDrive` rejects a restore with `session_file_conflict` whenever the local rollout file hash differs from the remote file hash, even when the two files are prefix-compatible extensions of the same Codex session.
- A Codex rollout file grows by appending JSONL lines, so a mismatch is not always divergence — it can mean one side has more turns than the other.
- The local relocation path already handles this correctly via `updateCodexDestinationIfSameSessionExtends` (`bytes.HasPrefix` check), but the Drive restore path did not apply the same logic.
- Two legitimate cases were incorrectly blocked: remote is newer than local (should overwrite local), and local is ahead of remote (should keep local unchanged).

### Current Ask

- Apply the same prefix-extension semantics to the Drive restore conflict check that the local relocation path already uses.

### Key Decisions

- `D-1` Mirror the `bytes.HasPrefix` logic from `updateCodexDestinationIfSameSessionExtends` into the restore path so prefix-compatible extension is accepted, not rejected.
- `D-2` Only return `session_file_conflict` when the bytes are genuinely divergent (neither side is a prefix of the other).

### Constraints

- The fix must not weaken protection against genuinely divergent files — two different session chains that happen to share the same id string must still be rejected.
- The fix applies to the Codex rollout file restore path; Claude and Gemini providers are not affected by this code path.

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
- `F-2` Replace the single hash-equality conflict guard with a three-branch check:
  - if `hashBytesSHA256(existing) == manifest.ProviderFile.SHA256` → identical, no write
  - else if `bytes.HasPrefix(providerBytes, existing)` → remote extends local, overwrite local
  - else if `bytes.HasPrefix(existing, providerBytes)` → local already extends remote, keep local
  - else → genuinely divergent, return `session_file_conflict`

## 8. Validation

- `V-1` Existing restore and sync tests pass: `go test ./internal/runner/... -run "TestRestore|TestChatSession|TestSync"` — 36 tests pass.
- `V-2` Code review: genuinely divergent files (different first JSONL line) still return `session_file_conflict`; only prefix-compatible pairs are accepted.

## 9. Regression Guard

- tests: existing 36 restore/sync tests cover the identical-hash path and the not-found path; new prefix-extension paths are covered by the symmetry with `updateCodexDestinationIfSameSessionExtends` tests in `cross_account_resume_test.go`
- alerts: `session_file_conflict` errors in Drive restore logs should decrease after this fix
- audit checks: verify that a re-restore of a continued chat no longer returns `session_file_conflict`

## 10. Follow-Up Document Updates

- upstream docs that must change: SD-14 §Q-3b updated in this session to document the gap and the intended fix — now resolved
- notes left unchanged on purpose: the design rule "never silently merge genuinely divergent rollout states" is preserved; only the definition of "conflict" is narrowed to exclude prefix-compatible extensions
