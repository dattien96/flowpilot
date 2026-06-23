# Task-088: Desktop Child Agent Stop Button

## Metadata

- Document ID: `Task-088`
- Title: `Desktop Child Agent Stop Button`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md), [Task-083: Desktop Agents Panel And Focus Navigation](./Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md)
- Child Documents: `None`
- Related Documents: [Task-087: Chat Slash Commands For Skill And Agent UI](../done/Task-087-Chat-Slash-Commands-For-Skill-And-Agent-UI.md), [BUG-133](../../09-BugFix/done/BUG-133-Block-Main-Run-On-Running-Wait-True-Child.md), [CA-122](../../../change-audit/CA-122-child-agent-stop-button.md)
- Replaces: `None`
- Tags: `multi-agent, desktop, react, chat-input, ux`

## AI Quick View

### Summary

- When the child-agent chat view is active (`childRunFocused=true`) and the focused child is running, a Stop button now appears to the left of the Main button.
- The Stop button reuses the existing `send-btn-stop` style and `StopIcon`, and calls the existing `stop()` store action which calls `client.interrupt(runId)` — where `runId` is already set to the focused child's run ID by `focusAgentRun`.
- No new store actions, no new client methods, and no CSS were required.

### Current Ask

- Surfaced the Stop button for child agents in the read-only child-agent view so users can interrupt a running child without having to return to the main chat first.

### Key Decisions

- `T-1` Re-use `stop()` directly: when a child agent is focused, `store.runId` is already the child's run ID (set by `focusAgentRun`), so calling `stop()` interrupts the correct run without any additional routing logic.
- `T-2` Gate visibility on `blocked` (status `running | waiting_approval | waiting_question`), matching the same condition used for the main-chat Stop button, so the button disappears automatically when the child is idle.
- `T-3` Button is inserted between the input-note wrapper and the existing Main button; both are visible simultaneously when the child is running.

### Constraints

- Must not break the single-agent (no child) experience — the change is inside the `childRunFocused` branch only.
- No new store state or client contract changes allowed.

### Open Questions

- None.

### Source Refs

- `CP-19` P-10 (stop semantics for child agents deferred from Task-083/084); Task-083 `T-2` (`focusAgentRun` sets `runId`); `ChatInput.tsx` lines 1024–1035; `store.ts` `stop()` line 793.

## 1. Goal

Allow users to stop a running child agent from the child-agent transcript view without navigating back to the main chat.

## 2. Parent Links

- coding plan: [CP-19](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- tech design: [SD-16: Agent Spawn And Tool Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: `CP-19 P-10`; `Task-083 T-2`

## 3. Trigger

The child-agent chat view showed a read-only "transcript only" banner with only a Main button — no way to stop the child from that view. Users had to return to the main chat and use the orchestration board or the Stop button there, adding unnecessary friction.

## 4. Exact Change

- `T-1` In `ChatInput.tsx`, inside the `childRunFocused` branch (render block at line 1024), add a conditional `<button className="btn send-btn send-btn-stop">` that renders only when `blocked === true`. The button calls `() => void stop()` and carries `aria-label="Stop child agent"`.
- `T-2` No changes to `store.ts`, `HttpWsRunnerClient.ts`, `contract.ts`, or CSS files.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/components/ChatInput.tsx`
- modules: `ChatInput` component — `childRunFocused` render branch
- routes: n/a
- tables: n/a

## 6. Acceptance Check

- Open a child agent chat view while the child is actively running → Stop button is visible to the left of the Main button.
- Click Stop → child agent is interrupted; `blocked` becomes false and Stop button disappears.
- When child is idle/completed → Stop button is not rendered; only Main button is shown.
- Single-agent chat (no child agent focused) is unaffected.

Note: acceptance could not be exercised in a browser preview during this session because it requires live child-agent running state; behavioral correctness was verified by code inspection — `stop()` already calls `client.interrupt(runId)` and `runId` is set to the child's ID by `focusAgentRun`.

## 7. Out of Scope

- Stopping the entire agent loop from the child view (that is `stopAgentLoop`, available on the orchestration board).
- Queuing or re-starting a stopped child from this view.
- Any new backend endpoint or store action.

## 8. Completion Notes

- result: Implemented and merged. One conditional button added, three lines of JSX.
- follow-ups: None required; the Stop-from-child-view gap described in CP-19 P-10 is now closed for Phase 1.
- upstream docs updated: No upstream business rules or design constraints changed; this is a pure UI gap-fill within the already-designed child-focus navigation (Task-083).
