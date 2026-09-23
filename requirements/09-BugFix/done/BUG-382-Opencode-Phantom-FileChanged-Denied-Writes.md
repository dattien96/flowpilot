# BUG-382: Opencode phantom `file_changed` on `in_progress`/`failed`/permission-rejected tool updates poisons gate `WrittenPaths`

## Metadata

- Document ID: `BUG-382`
- Title: `mapOpencodeToolCallUpdate ignores update status — denied writes emit file_changed → false r-ca/r-contract violations`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-57-Opencode-Provider-Integration](../../07-Coding-Plan/done/CP-57-Opencode-Provider-Integration.md)
- Feature Keys: `ai-providers, context-regression-engine`

## AI Quick View

### Summary

- `mapOpencodeToolCallUpdate` (`apps/local-runner/internal/runner/opencode_event_mapper.go:67-92`) appends `EventFileChanged` for every `tool_call_update` whose tool/kind looks like a mutation — keyed on the *declared* path, with NO check of `status` (`in_progress`, `failed`, `pending`) or permission outcome.
- A permission-rejected write therefore emits `file_changed` twice (once on the `in_progress` update, once on the `failed` update) → `gate_hook.go:728` registers the path in `WrittenPaths` → `r-ca`/`r-contract` violations on a turn that changed nothing.
- Same mapper also emits `tool_completed` for non-terminal updates (`status:"in_progress"` surfaces as completed).
- Opposite polarity of BUG-375: devin under-emits `file_changed` (never); opencode over-emits (on denied/failed writes).

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.
- Fix direction noted by tester: only emit `EventFileChanged` when `status` indicates success/completed (skip `in_progress`, `failed`, `pending`); emit `tool_completed` only on terminal statuses.

## Bug report

- **Symptom**: A denied write produces `file_changed` events and a subsequent `flow_gate_violation` ("code changed but no change-audit note found; … code changed without a declared Change Contract") even though nothing was written to disk.
- **Expected**: `file_changed` only when the mutation actually applied (tool status completed/success, permission granted); a denied or failed write produces no file event.
- **Actual** (cp57 run-29, scan posture — writes auto-denied by design): seq 20 `tool_completed status:"in_progress"`; seq 21 `file_changed path=…/scan-test.txt changeType:"edit"`; seq 22 `tool_completed status:"failed"` (permission rejected); seq 23 `file_changed` again; seq 28 `flow_gate_violation` r-ca/r-contract. `ls …/scan-test.txt` → No such file.
- **Impact**: medium — false-positive gate violations/reprompts on denied writes; gate `WrittenPaths` untrustworthy for opencode; also misleading `tool_completed` status `in_progress` in timelines.

## Reproduction

1. Opencode chat run (cp57 :19257, run-29) with `chatPosture:"scan"` (read-only: writes auto-denied, never asked).
2. Prompt: create `scan-test.txt`.
3. Opencode attempts the write → bridge auto-denies → `tool_call_update` arrives `status:"failed"`, output `{"error":"The user rejected permission to use this specific tool call."}`.
4. Observe SSE `l57_posture_sse.ndjson` seq 19-28 → phantom `file_changed` ×2 + gate violation; file never exists on disk.

## Root cause

- `apps/local-runner/internal/runner/opencode_event_mapper.go:67-92` — `mapOpencodeToolCallUpdate` builds `EventToolCompleted` unconditionally and appends `EventFileChanged` whenever `opencodeToolMutationKind(update) != ""`; `status` (read at :68) is used only for the tool-completed status string, never to gate file-event emission.
- Downstream: `gate_hook.go:728` treats `EventFileChanged` as `WrittenPaths` → `flowgate/evaluate.go` `r-ca`/`r-contract` fire on a turn that changed nothing.
- Extends the CP-57 §E watch item "file_changed đến trước permission_required" — that note covered ordering; live evidence shows emission on `failed` status and real gate impact.

## Evidence

- `~/fp-beds/lt-evidence/cp57/BUG-LIVE-57-3.md`, `RESULT.md` (posture scan row), `l57_posture_sse.ndjson` (seq 19-28), `l57_run29_events.json`, `runner.log` (permission denied → replay recovery → gate violation window); `ls` proof `scan-test.txt` absent.

## Severity

- medium

## Completion Notes (implemented 2026-09-23, CA-917)

- Fix: `mapOpencodeToolCallUpdate` gates on mapped status — non-terminal
  statuses emit nothing (no premature tool_completed); `file_changed` only
  when status maps to success. Denied writes (status:"failed") emit
  tool_completed(failed) with no file_changed.
- Tests: `bug382_opencode_phantom_filechanged_test.go` (denied, in_progress,
  completed regression guard).
- Live: cp57 denied-write frame shape pinned in test.
