# BUG-127: Codex Rollout Migration Rename Fails On Windows

## Metadata

- Document ID: `BUG-127`
- Title: `Codex Rollout Migration Rename Fails On Windows`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `self-review`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md), [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- Child Documents: `None`
- Related Documents: [BUG-124: Codex Spawn-Agent Reserved Tool Name](./BUG-124-Codex-Spawn-Agent-Reserved-Tool-Name.md), [CA-118: Fix Codex Rollout Migration Rename On Windows](../../../change-audit/CA-118-fix-codex-rollout-migration-rename-on-windows.md)
- Replaces: `None`
- Tags: `codex, resume, windows, file-io, regression, runner`

## AI Quick View

### Summary

- BUG-124's `migrateCodexReservedSpawnAgentTool` rewrites a legacy rollout file via temp-file + `os.Rename`, but it holds the source file handle open (`defer src.Close()`) when the rename runs.
- On Windows, renaming/replacing a file with an open handle fails with `Access is denied`, so every legacy Codex chat resume that triggers the migration fails the turn on Windows.
- A secondary, test-only Windows issue: the migration test asserted a hardcoded `0640` file mode, which Windows reports as `0666` regardless of create mode.
- Fix: close the source handle after the copy and before the rename; assert the migrated mode equals the original file's OS-reported mode instead of a hardcoded constant.

### Current Ask

- Make BUG-124's Codex rollout migration work on Windows, and make its regression test platform-agnostic, without changing the migration's behavior on Unix.

### Key Decisions

- `V-1` The source handle must be released before `os.Rename` replaces the original file (Windows constraint), after the reader has been fully drained into the temp file.
- `V-2` The mode-preservation assertion compares the migrated file's mode to the original file's OS-reported mode, not a hardcoded `0640`.

### Constraints

- Do not change the atomic temp-file + rename strategy, the migrated content, or Unix behavior.
- The close must be idempotent so the existing `defer` on early-return paths remains correct.

### Open Questions

- None.

### Source Refs

- Windows failure: `codex_appserver_test.go:603` — `rename …jsonl.bug124-<n> …jsonl: Access is denied`.
- `apps/local-runner/internal/runner/session_file_locator.go` — `migrateCodexReservedSpawnAgentTool`.
- `apps/local-runner/internal/runner/codex_appserver_test.go` — `TestCodexAdapterResumeMigratesLegacySpawnAgentTool`.

## 1. Issue Summary

The BUG-124 migration opens the legacy rollout file (`os.Open(path)` with `defer src.Close()`), streams its remaining records into a sibling temp file, then `os.Rename(tmp, path)`. Because `defer` runs only at function return, the handle to `path` is still open at rename time. Windows refuses to replace a file that has an open handle, returning `Access is denied`, which surfaces as `prepare codex session for resume: rename … Access is denied` and fails the resumed turn.

## 2. Parent Links

- impacted coding plan: [CP-19](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- impacted tech design: [SD-16](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- impacted system spec: [SS-11](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: Windows desktop FlowPilot local runner, Codex provider, a legacy rollout whose `session_meta.dynamic_tools` still names `spawn_agent`.
- reproduction steps:
  1. On Windows, run `cd apps/local-runner && go test ./internal/runner/ -run TestCodexAdapterResumeMigratesLegacySpawnAgentTool -count=1`.
  2. Observe `rename …jsonl.bug124-<n> …jsonl: Access is denied`.
  3. Equivalent product path: resume any legacy Codex chat that needs the migration.
- frequency: always on Windows; never on Unix (which permits replacing an open file).

## 4. Expected vs Actual

- expected: the migration atomically rewrites the rollout file and the resumed Codex turn proceeds on every OS.
- actual: on Windows the rename fails with `Access is denied` and the resume turn fails.

## 5. Impact

- users affected: Windows users resuming legacy Codex chats (the primary development platform here).
- workflows affected: Codex history reopen / resumed turns that trigger the BUG-124 migration.
- severity: High on Windows — the resumed turn fails before model execution.

## 6. Root Cause

- hypothesis: a still-open source handle blocks the Windows rename.
- confirmed cause: `migrateCodexReservedSpawnAgentTool` relied on `defer src.Close()`, so the original file's handle was open when `os.Rename(tmp, path)` ran. Windows `MoveFileEx` cannot replace a file with an open handle. The test additionally hardcoded a `0640` mode that Windows cannot represent.
- evidence: stashing the fix reproduces 15 failures including the migration test; applying the fix leaves exactly the 14 pre-existing environment failures and the migration test passes. The error message names the rename of the temp file over the original.

## 7. Fix Strategy

- `F-1` Replace `defer src.Close()` with an idempotent `closeSrc()` helper; keep `defer closeSrc()` so early-return paths still close the handle.
- `F-2` Call `closeSrc()` after `io.Copy` has drained the reader into the temp file and before `os.Rename`, releasing the handle on the success path.
- `F-3` In the test, capture the original file's OS-reported mode after creation and assert the migrated file equals it, instead of asserting a hardcoded `0640`.

## 8. Validation

- `V-1` `go test ./internal/runner/ -run TestCodexAdapterResumeMigratesLegacySpawnAgentTool -count=1` on Windows — pass (was `Access is denied`).
- `V-2` `go test ./internal/runner/ -run 'TestCodexAdapter|TestMapCodexNotification' -count=1` — pass.
- `V-3` No-regression check: full `go test ./internal/runner/ -count=1` failure set is identical with and without this fix except that the migration test now passes (14 pre-existing environment failures remain: real-`codex`-binary resume tests, google-drive proxy path, provider-home skill merges — none touch the migration path).
- `V-4` `go build ./internal/runner/...` — pass.

## 9. Regression Guard

- tests: `TestCodexAdapterResumeMigratesLegacySpawnAgentTool` now runs to completion on Windows and asserts mode preservation cross-platform.
- alerts: any `Access is denied` during Codex resume indicates a re-introduced open-handle-before-rename.
- audit checks: the migration must close the source before renaming over it.

## 10. Follow-Up Document Updates

- upstream docs that must change: none; this is a platform-correctness delta to BUG-124, not a behavior change.
- notes left unchanged on purpose: the atomic temp-file + rename approach and the migrated content are unchanged; only handle lifetime and the test's mode assertion were corrected.
