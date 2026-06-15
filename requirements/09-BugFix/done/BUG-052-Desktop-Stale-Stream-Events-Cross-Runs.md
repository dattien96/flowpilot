# BUG-052: Desktop Stale Stream Events Cross Runs

## Metadata

- Document ID: `BUG-052`
- Title: `Desktop stale stream events cross runs`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot maintainers`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: `requirements/08-Task/done/Task-037-Desktop-Project-Run-History-Popover.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-046-Desktop-History-Replay-Loses-User-Prompts.md`, `requirements/09-BugFix/done/BUG-050-Desktop-Claude-Provider-Blank-Chat.md`, `requirements/09-BugFix/done/BUG-051-Claude-Limit-Reported-As-Login.md`, `change-audit/CA-062-fix-desktop-stale-run-stream-events.md`
- Replaces: `none`
- Tags: `desktop, local-runner, run-history, streaming, ui, regression`

## AI Quick View

### Summary

- A still-running Codex stream could render its assistant response into a different visible run after the user switched back to a Claude run.
- Refreshing or reopening the run removed the stray response, proving persisted run data and replay logic were correct.
- The desktop store used one global timeline and folded every active async stream into that global timeline.
- The store did not verify that a stream still belonged to the currently visible `runId` before applying events.

### Current Ask

- Do not render generated responses from another run in the currently visible run UI.

### Key Decisions

- `F-1` Pass the expected run id into every desktop stream consumer.
- `F-2` Add a small run-identity guard before applying provider events to the timeline.
- `F-3` Stop stale stream folding when the visible run changes instead of mutating the wrong timeline.
- `V-1` Add a focused reducer-level regression guard for active-run event matching.

### Constraints

- GitNexus MCP tools were not exposed in this thread, so symbol impact checks were performed with local call-graph search and targeted tests.
- The runner and persisted event replay should remain unchanged because the user observed refresh/reopen already corrects the UI.
- Existing `startRun` failures must still surface in the active timeline even before a run id exists.

### Open Questions

- A future desktop store could track active streams per run, but this bug only requires preventing stale stream mutation of the active run UI.

### Source Refs

- User report on `2026-06-15`: Codex response appears in the Claude run UI when switching back before Codex finishes.
- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/desktop-flowpilot/src/state/timelineReducer.ts`
- `apps/desktop-flowpilot/src/state/timelineReducer.test.ts`

## 1. Issue Summary

When a user starts a Claude run, starts a separate Codex run, and immediately switches back to the Claude run before Codex finishes, the live Codex response can appear inside the Claude run timeline. Switching runs again or refreshing removes it because the stored/replayed run data is correct.

## 2. Parent Links

- impacted coding plan: `requirements/08-Task/done/Task-037-Desktop-Project-Run-History-Popover.md`
- impacted tech design: `requirements/10-Refactor/New-System/03-Solution-And-System-Design.md`
- impacted system spec: unknown

## 3. Environment and Reproduction

- environment: Desktop FlowPilot UI connected to local runner on `2026-06-15`.
- reproduction steps:
  1. Start a run with Claude.
  2. Start a new run with Codex.
  3. Before Codex returns, reopen or switch back to the Claude run.
  4. Wait for Codex to stream a response.
- frequency: Reproducible while an older stream remains active after the visible run changes.

## 4. Expected vs Actual

- expected: The visible Claude run timeline only displays events for the Claude run.
- actual: The in-flight Codex stream can append its response to the currently visible Claude timeline until the view is refreshed or reopened.

## 5. Impact

- users affected: Desktop users switching between active runs while a provider is still streaming.
- workflows affected: Multi-provider run comparison, history reopen, live run monitoring.
- severity: `medium`, because persisted data remains correct but the live UI temporarily shows misleading output in the wrong run.

## 6. Root Cause

- hypothesis: The runner mixed run events across streams.
- confirmed cause: The desktop store maintained a single global `timeline` and `runId`. `consumeStream()` folded each active async provider stream into that global state without checking whether the stream's run id still matched the currently visible run id. When `openHistoryRun()` changed `runId` and cleared/rebuilt the timeline, an older Codex stream could still call `set((s) => applyEvent(s, e))` against the new visible Claude state.
- evidence: The stray response disappears after switching or refresh, which replays persisted events for the selected run. Local call-graph search showed `consumeStream()` is shared by `sendPrompt()`, `reconnect()`, and `openHistoryRun()`.

## 7. Fix Strategy

- `F-1` Added `shouldApplyRunEvent(activeRunId, streamRunId)` to centralize the active-run match check.
- `F-2` Passed the expected run id into `consumeStream()` from `sendPrompt()`, `reconnect()`, and `openHistoryRun()`.
- `F-3` Stopped stream consumption before applying an event when the visible run id no longer matches the stream's run id.
- `F-4` Guarded existing-run send errors so late failures do not mutate a different active run, while preserving visible errors for failed `startRun()` calls that never received a run id.

## 8. Validation

- `V-1` `npm run typecheck` from `apps/desktop-flowpilot` passed.
- `V-2` `npx tsc src/state/timelineReducer.ts src/state/timelineReducer.test.ts src/types/contract.ts --outDir C:/working/flowpilot/.tmp-desktop-reducer-test --module NodeNext --moduleResolution NodeNext --target ES2022 --lib "ES2022,DOM" --strict --esModuleInterop --skipLibCheck` passed.
- `V-3` `node --test C:/working/flowpilot/.tmp-desktop-reducer-test/state/timelineReducer.test.js` passed.

## 9. Regression Guard

- tests: `timelineReducer.test.ts` now verifies events only apply when the stream run id matches the active run id.
- alerts: none added.
- audit checks: `change-audit/CA-062-fix-desktop-stale-run-stream-events.md` records the change.

## 10. Follow-Up Document Updates

- upstream docs that must change: none; this is a desktop store isolation fix for an existing stream/replay flow.
- notes left unchanged on purpose: no runner event-contract changes were made because persisted replay already behaved correctly.
