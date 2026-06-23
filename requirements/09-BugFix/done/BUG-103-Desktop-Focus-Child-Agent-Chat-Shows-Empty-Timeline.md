## Metadata

- Document ID: `BUG-103`
- Title: `Desktop Focus Child Agent Chat Shows Empty Timeline`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-20`
- Last Updated: `2026-06-20`
- Parent Documents: [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md), [SS-06: Workflow Skill Agent](../../05-System-Specs/SS-06-Workflow-Skill-Agent.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: none
- Related Documents: [BUG-099: Desktop Spawn Agent Does Not Surface Child Runs In Agents Panels](./BUG-099-Desktop-Spawn-Agent-Does-Not-Surface-Child-Runs-In-Agents-Panels.md), [BUG-102: Desktop History Open Hangs On Open Ended Spawn Agent Replay Stream](./BUG-102-Desktop-History-Open-Hangs-On-Open-Ended-Spawn-Agent-Replay-Stream.md)
- Replaces: none
- Tags: desktop, agents, child-chat, replay, regression, severity-high

## AI Quick View

### Summary

- The main chat could reopen correctly, but clicking a child agent opened a blank chat timeline.
- The desktop focused the child run and cleared the timeline, then sometimes skipped the child replay because it reused a replay cursor without a cached child snapshot.
- Follow-up evidence showed `event stream failed: 404 (HTTP 404 / stream_failed)`: the child summary could be listed while the child run itself was not hydrated in the runner's in-memory `runs` map.
- The fix hydrates the child with `resumeRun(childRunId)` before opening its event stream, then replays from seq 0 when no cached child snapshot exists.

### Current Ask

- Capture and fix the child agent focus timeline bug.

### Key Decisions

- `V-1` A child timeline without a cached snapshot must replay from seq 0.
- `V-2` A child timeline with a cached snapshot may resume from the last replay cursor to avoid duplicate events.
- `V-3` The desktop must call `resumeRun(childRunId)` before opening `/events/stream` so persisted child runs are reconstructed before focus.
- `V-4` If the child event stream cannot load, show an explicit error message instead of an empty chat.

### Constraints

- Do not change the agent graph contract.
- Do not duplicate child events when the user has already focused the child and a local snapshot exists.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/desktop-flowpilot/src/state/store.test.ts`
- `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`
- `apps/desktop-flowpilot/src/types/contract.ts`
- `apps/local-runner/internal/runner/interactive_handlers.go`
- `apps/local-runner/internal/runner/interactive_resume.go`

## 1. Issue Summary

After reopening the main chat, the user could see the parent conversation, but clicking the child agent chat showed an empty timeline. After the first UI fallback was added, the concrete failure became `event stream failed: 404 (HTTP 404 / stream_failed)`.

## 2. Parent Links

- impacted coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- impacted tech design: [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- impacted system spec: [SS-06: Workflow Skill Agent](../../05-System-Specs/SS-06-Workflow-Skill-Agent.md)

## 3. Environment and Reproduction

- environment: desktop FlowPilot, parent run with at least one child agent
- reproduction steps:
  1. Open the main chat from history.
  2. Click the child agent row in the Agents panel or timeline banner.
  3. Observe the child chat timeline.
- frequency: reproducible when the desktop lists a persisted child summary before the runner reconstructs the child run for `/events/stream`

## 4. Expected vs Actual

- expected: first focus of a child after reopening a parent chat replays the full child transcript
- actual: the child chat opened empty, then showed `event stream failed: 404 (HTTP 404 / stream_failed)` once stream errors were surfaced

## 5. Impact

- users affected: users reviewing child agent work
- workflows affected: spawn-agent review, child debugging, approval/history inspection
- severity: high, because child work appears lost even when the child run exists

## 6. Root Cause

- hypothesis: the desktop switched to the child run but replayed from the wrong cursor
- confirmed cause:
  - `focusAgentRun` used `_runReplaySeq[childRunId]` even when there was no cached child snapshot. That can skip all child events while the visible child timeline starts empty.
  - For persisted children, the Agents list can be built from session metadata while `/events/stream` still returns 404 because `s.runs[childRunId]` has not been reconstructed. Parent history open hydrates the parent only; child focus also needs to hydrate the child.
- evidence: the runner `resumeRun` path reconstructs a persisted run and seeds its transcript from disk before streaming; the desktop now calls `resumeRun(childRunId)` before opening the child focus stream.

## 7. Fix Strategy

- `F-1` In `focusAgentRun`, use the child replay cursor only when a child snapshot exists.
- `F-2` When there is no child snapshot, replay the child stream from the beginning.
- `F-3` Catch child stream failures and append a visible system error so the child chat is never silently blank.
- `F-4` Call `client.resumeRun(childRunId)` before opening the child event stream.
- `F-5` Use a cancellable child focus stream so stale focused-child streams do not survive navigation.

## 8. Validation

- `V-1` `npm run typecheck` passes in `apps/desktop-flowpilot`.
- `V-2` `npx tsc -p ../../tsconfig.phase1-tests.json` passes in `apps/desktop-flowpilot`.
- `V-3` Added `focusAgentRun replays from start when no child snapshot is cached`.
- `V-4` Updated cursor regression coverage with `focusAgentRun resumes from the last replay cursor when a child snapshot is cached`.
- `V-5` Added `focusAgentRun resumes the child run before opening its event stream`.

## 9. Regression Guard

- tests: focused store tests for child hydration-before-stream, uncached child replay, and cached child cursor replay
- alerts: none
- audit checks: a blank child timeline should only be possible when the child genuinely has no events; stream failures must render an error

## 10. Follow-Up Document Updates

- upstream docs that must change: none
- notes left unchanged on purpose: agent graph and runner SSE contracts remain unchanged
