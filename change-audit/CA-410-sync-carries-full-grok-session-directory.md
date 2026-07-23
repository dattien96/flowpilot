# CA-410: chat-sync manifest carries the whole Grok session directory, not just chat_history.jsonl

## Summary

Operator restored a deleted Grok chat from Drive — it opened fine, but the
very next turn failed with a 3x recoverable-error retry loop then a hard
`"Path not found."`. Root cause: Grok's own `session/load` ACP call is a
provider-owned stateful resume that reads the WHOLE session directory
(`chat_history.jsonl`, `events.jsonl`, `updates.jsonl`, `prompt_context.json`,
`system_prompt.txt`, `summary.json`, `signals.json`, `resources_state.json`,
`rewind_points.jsonl`, `announcement_state.json`) — but the sync manifest
only ever carried `chat_history.jsonl` (BUG-310 taught the read side that
Grok's path is a directory, but never extended to carrying the rest of it).
Deleting a chat deletes the provider's own session directory outright, so
every restored Grok chat's directory came back with exactly one file. Full
analysis in
[BUG-316](../requirements/09-BugFix/done/BUG-316-Restored-Grok-Chat-Fails-Next-Turn-Session-Directory-Incomplete.md).

## Change

- `ChatSessionSyncManifest` += `ProviderFiles []ChatSessionFile`
  (`json:"providerFiles,omitempty"`; Grok-only) + unexported
  `grokSidecarBodies map[string][]byte` (never reaches `manifest.json` —
  `encoding/json` skips unexported fields, so `BuildChatSessionSyncManifest`'s
  public 3-value return signature stays untouched — 8 pre-existing tests call
  it directly on that exact arity).
- New `resolveGrokSessionSidecarFiles` lists a Grok session directory and
  reads every regular file except `chat_history.jsonl` (already handled) and
  `*.lock` markers (zero-byte runtime locks, never synced).
- `BuildChatSessionSyncManifest` attaches sidecars only for
  `ProviderKeyGrok` with a real (non-`thread-`) session id.
- `uploadChatSessionRunFiles` uploads each sidecar into the same per-run
  provider folder as the primary file.
- `restoreChatRunTreeFromDrive` restores each sidecar into the same directory
  as the primary file, with identical strictness (missing →
  `sync_remote_not_found`; hash mismatch → `sync_integrity_failed`; different
  local copy → `session_file_conflict`; identical local copy → no-op) —
  reuses the primary file's already-validated, cwd-recomputed target
  directory (BUG-312) rather than adding a second path-validation path.

Provider parity: sidecar collection/restore only ever fires for
`ProviderKeyGrok`; a dedicated test asserts Codex/Claude manifests never
populate `ProviderFiles`.

## additive-tests-only compliance

New test file (`bug316_grok_restore_session_dir_test.go`, 4 test functions +
3 small helpers) only. No existing test touched. Verified via a full
pre-existing chat-sync/BUG-310/BUG-311 suite re-run (39/39 pass, unmodified).

## Verification

- Red-first TDD: the 3 repro tests fail on pre-fix HEAD (`git stash` isolating
  only `chat_session_sync.go`, new test file kept in place) — `manifest
  providerFiles = <nil>`, `restored session dir missing "events.jsonl"`, and
  "restore must conflict [but didn't]" respectively — then pass after popping
  the stash back. Assertions read raw manifest JSON / on-disk bytes, not new
  Go symbols, so they compile on the baseline.
- `TestRestoreChatRunFromDriveRebuildsGrokSessionSidecarFiles`: full round
  trip — sync a 10-file session directory, delete it entirely (matching a
  real chat delete), restore under a DIFFERENT target cwd, every sidecar file
  byte-identical to the original at the newly-recomputed location.
- `TestRestoreChatRunFromDriveGrokSidecarConflictsWithDifferentLocalCopy`:
  restore onto a directory with identical sidecars is a no-op; restore after
  a local sidecar is corrupted hard-conflicts (`session_file_conflict`), never
  silently overwrites a file a live process might be using.
- `TestSyncChatRunToDriveNeverCarriesSidecarFilesForCodexOrClaude`:
  cross-provider guard — Codex/Claude manifests never populate
  `providerFiles`.
- `go build ./...`, `go vet ./internal/runner/...`: clean. Targeted sweep
  (every `TestBug31[2-6]*`, `TestSyncChat*`, `TestRestoreChat*`,
  `TestBuildChatSessionSyncManifest*`, `TestRestoreTargetPath*`,
  `TestRestoreSessionFile*`): fully green.
- Full-package sweep (`go test ./...`): fix = 2496 passed / 17 failed;
  `git stash` baseline (fix removed) = 2494 passed / 16 failed. The one extra
  failure on the fix run, `TestFinalizerHookSurfacesArtifacts`, passed 3/3
  when re-run in isolation on the fixed tree right after — a pre-existing
  order-dependent flake (already catalogued this session alongside
  `TestRun20332FlowHubHistoryParityForEveryProvider/grok`), not caused by this
  change. Every other failure is the identical previously-catalogued
  machine/CLI/env-dependent set on both runs.
- Live end-to-end (running dev runner, project Gate-sandbox): created a real
  Grok chat, sent one turn, synced to Drive, **deleted it** (confirmed the
  entire session directory — 12 files — was gone from disk, not just
  `chat_history.jsonl`), restored (all 9 non-lock files rebuilt, sizes
  byte-matching the originals), resumed, sent a second live turn — Grok
  streamed a real reply and the turn completed normally. Before the fix this
  exact sequence reproduced `"Path not found."` 100% of the time.

## Known limits (documented, out of scope)

- Chats synced by a pre-fix manifest still restore with only
  `chat_history.jsonl` until re-synced from a machine that still holds the
  full session directory (mirrors BUG-313's identical caveat for the turn
  log).
- `.lock` files are never synced by design (V-1 in BUG-316) — this is
  intentional, not a gap.

# ---8<--- flowpilot:change-ledger
feature_key: google-drive
source_doc_id: BUG-316
change_type: bugfix
summary: chat-sync manifest now carries every file in a Grok session directory (not just chat_history.jsonl) and restore rewrites them all, so a restored Grok chat's next turn resumes instead of failing FS_NOT_FOUND ("Path not found.").
# --->8---
