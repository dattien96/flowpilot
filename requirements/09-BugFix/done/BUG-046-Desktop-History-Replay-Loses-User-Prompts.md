# BUG-046: Desktop History Replay Loses User Prompts

## Metadata

- Document ID: `BUG-046`
- Title: `Desktop History Replay Loses User Prompts`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-13`
- Last Updated: `2026-06-13`
- Parent Documents: `requirements/08-Task/done/Task-037-Desktop-Project-Run-History-Popover.md`
- Child Documents: `change-audit/CA-058-history-replay-prompts.md`
- Related Documents: `change-audit/CA-057-desktop-project-run-history-popover.md`, `change-audit/CA-058-history-replay-prompts.md`
- Replaces: `none`
- Tags: `desktop, history, replay, regression, local-runner`

## AI Quick View

### Summary

- Opening an old desktop run from the History popover rebuilt the timeline without user prompt bubbles.
- Live sends showed prompts because the desktop store appended them locally before streaming.
- History replay clears local timeline state and rebuilds only from persisted runner events.
- The persisted `turn_started` event did not include the submitted prompt, so replay had no source for user prompts.

### Current Ask

- Preserve and render user prompts when reopening old runs through desktop history.

### Key Decisions

- `V-1` Persist the submitted prompt on `turn_started` so run replay has a durable prompt boundary.
- `V-2` Render replayed `turn_started.prompt` in the desktop timeline while avoiding duplicate prompt bubbles during live sends.
- `V-3` Keep the change backward-compatible by making the desktop `turn_started.prompt` field optional.

### Constraints

- Existing historical events created before this fix may still lack `prompt`; the UI can only restore prompts for runs whose replay stream contains the field.
- The fix must not alter provider execution input or retry behavior.
- The runner event contract is shared by HTTP, mock, replay, and tests.

### Open Questions

- None for this bug fix.

### Source Refs

- User report: `$add-new-bug when access old run via history view, all prompts from users lost, only the response remain`
- `apps/local-runner/internal/runner/interactive_service.go`
- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/desktop-flowpilot/src/types/contract.ts`
- `apps/desktop-flowpilot/src/client/MockRunnerClient.ts`

## 1. Issue Summary

When a user opens a previous desktop run from the History popover, the timeline shows assistant responses and tool events, but the user prompts are missing.

This makes old runs hard to understand because the response is detached from the prompt that caused it.

## 2. Parent Links

- impacted coding plan: `requirements/08-Task/done/Task-037-Desktop-Project-Run-History-Popover.md`
- impacted tech design: `requirements/10-Refactor/New-System/03-Solution-And-System-Design.md`
- impacted system spec: unknown

## 3. Environment and Reproduction

- environment: desktop FlowPilot UI using local-runner history replay
- reproduction steps:
  1. Start a desktop run.
  2. Send one or more prompts.
  3. Open the History popover.
  4. Select the old run.
  5. Inspect the rebuilt timeline.
- frequency: consistent for replayed runs whose prompt existed only in desktop-local timeline state.

## 4. Expected vs Actual

- expected: the replayed timeline shows each user prompt followed by its thinking/response/tool activity.
- actual: the replayed timeline shows provider responses and events, but user prompts are absent.

## 5. Impact

- users affected: desktop users inspecting previous run history
- workflows affected: run review, debugging, comparison, and follow-up context recovery
- severity: medium UX/data-context regression

## 6. Root Cause

- hypothesis: desktop history replay omitted prompts because the replay endpoint did not include prompt events.
- confirmed cause: live sends appended prompt bubbles locally in `sendPrompt()`, but `openHistoryRun()` clears the timeline and rebuilds from persisted provider events. The persisted `turn_started` event did not carry the submitted prompt.
- evidence: `startTurn()` emitted `EventTurnStarted` without `Prompt`, while replay consumers depend on `streamRun(runId, 0)` events.

## 7. Fix Strategy

- `F-1` Add `Prompt: in.Prompt` when the local runner emits `EventTurnStarted`.
- `F-2` Extend the desktop `ProviderEventDTO` contract so `turn_started` may include `prompt`.
- `F-3` Update desktop `applyEvent()` to create a prompt timeline item from replayed `turn_started.prompt`.
- `F-4` Prevent duplicate prompt bubbles during live sends by detecting the already-pending local prompt item.
- `F-5` Update `MockRunnerClient` to emit the same prompt-bearing `turn_started` event shape.

## 8. Validation

- `V-1` `go test ./internal/runner -run TestNormalTurnPersistsWithSeq -count=1` passed.
- `V-2` `npm run typecheck` in `apps/desktop-flowpilot` passed.
- `V-3` `npm run build` in `apps/desktop-flowpilot` passed.
- `V-4` `npx gitnexus analyze` completed and `npx gitnexus status` reported the index up to date.

## 9. Regression Guard

- tests: `TestNormalTurnPersistsWithSeq` now asserts that live, persisted, and SSE-replayed `turn_started` events include the prompt.
- alerts: none.
- audit checks: `change-audit/CA-058-history-replay-prompts.md`

## 10. Follow-Up Document Updates

- upstream docs that must change: none; this is a compatible event-contract enrichment for replay.
- notes left unchanged on purpose: full `go test ./internal/runner -count=1` still has unrelated local environment failures for Google Drive proxy auth setup and missing `powershell`.
