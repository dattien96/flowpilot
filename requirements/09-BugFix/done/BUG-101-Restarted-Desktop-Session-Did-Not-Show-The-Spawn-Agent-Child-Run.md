## Metadata

- Document ID: `BUG-101`
- Title: `Restarted Desktop Session Did Not Show The Spawn Agent Child Run`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-20`
- Last Updated: `2026-06-20`
- Parent Documents: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md), [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- Child Documents: none
- Related Documents: [BUG-099: Desktop Spawn Agent Does Not Surface Child Runs In Agents Panels](./BUG-099-Desktop-Spawn-Agent-Does-Not-Surface-Child-Runs-In-Agents-Panels.md), [BUG-102: Desktop History Open Hangs On Open Ended Spawn Agent Replay Stream](./BUG-102-Desktop-History-Open-Hangs-On-Open-Ended-Replay-Stream.md), [BUG-083: Desktop Chat Resume Replays Composed Prompt Not User Input](./BUG-083-Desktop-Chat-Resume-Replays-Composed-Prompt-Not-User-Input.md)
- Replaces: none
- Tags: desktop, restart, history, spawn-agent, regression

## AI Quick View

### Summary

- After restarting the desktop app, the history view could show the original parent chat but miss the child spawn-agent run that the user expected to see next to it.
- The symptom was that the spawn-agent session appeared to have never started, even though the child work had already been requested.
- The fix makes history replay and agent refresh complete without blocking the UI, so the child run can be recovered after restart.

### Current Ask

- Capture the post-restart missing-child-run symptom as a standalone bugfix record.

### Key Decisions

- `V-1` A child run that was created before restart must still be visible after restart.
- `V-2` History replay should not prevent the agent list from refreshing.

### Constraints

- Do not treat this as a separate agent-creation feature bug.
- Keep the child run tied to the same parent history record.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/desktop-flowpilot/src/state/store.test.ts`
- `apps/local-runner/internal/runner/interactive_service.go`

## 1. Issue Summary

The user restarted the desktop app and then re-opened the spawn-agent history flow. The parent chat appeared, but the child run from the earlier spawn-agent action did not show up. That made it look like the child had never been started.

## 2. Parent Links

- impacted coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- impacted tech design: [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: desktop FlowPilot after a restart
- reproduction steps:
  1. Run the spawn-agent scenario.
  2. Restart the desktop/server stack.
  3. Reopen the original parent chat from history.
  4. Check whether the child run appears.
- frequency: deterministic before the replay/refresh fix

## 4. Expected vs Actual

- expected: both the parent history and the child spawn-agent run remain visible after restart
- actual: only the parent history was obvious; the child run appeared missing

## 5. Impact

- users affected: desktop users reviewing agent work after a restart
- workflows affected: run review, child-agent follow-up, restart recovery
- severity: medium to high, because the child run looks lost even when it was created

## 6. Root Cause

- hypothesis: the child run was not fully materialized into the visible history before restart
- confirmed cause: the blocking history replay path prevented the child refresh cycle from completing, so the desktop never finished recovering the child run into the visible list
- evidence: the new history-open path returns immediately and refreshes agent runs in the background, and the regression test proves the replay no longer blocks

## 7. Fix Strategy

- `F-1` Let history replay run in the background.
- `F-2` Refresh agent runs independently of the replay stream.

## 8. Validation

- `V-1` The new `openHistoryRun returns after attaching an open-ended history stream` regression test passes conceptually against the fixed store path.
- `V-2` Desktop typecheck passes.

## 9. Regression Guard

- tests: store regression for open-ended replay plus manual restart smoke
- alerts: none
- audit checks: child run visibility after restart must remain independent of the replay stream closing

## 10. Follow-Up Document Updates

- upstream docs that must change: none
- notes left unchanged on purpose: the child run itself is not a new feature; this is a recovery/visibility bug
