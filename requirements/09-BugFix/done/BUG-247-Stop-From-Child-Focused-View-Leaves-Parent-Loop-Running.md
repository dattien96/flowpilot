# BUG-247: Stop From Child-Focused View Leaves Parent Loop Running

## Metadata

- Document ID: `BUG-247`
- Title: `Stop From Child-Focused View Leaves Parent Loop Running`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-07`
- Last Updated: `2026-07-07`
- Parent Documents: [Task-088: Desktop Child Agent Stop Button](../../08-Task/done/Task-088-Desktop-Child-Agent-Stop-Button.md), [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `none`
- Related Documents: [CA-223: Cascade Main Stop To Running Child Agents](../../../change-audit/CA-223-parent-interrupt-cascades-to-child-agents.md), [BUG-133: Block Main Run On Running Wait-True Child](../done/BUG-133-Block-Main-Run-On-Running-Wait-True-Child.md)
- Replaces: `none`
- Tags: `agent-spawn, multi-agent, desktop, stop-control, regression`

## AI Quick View

### Summary

- Pressing Stop while viewing a focused child agent's read-only transcript only interrupted that child's turn; the parent run (and, when active, its agent loop) kept running until the user manually navigated back to the main chat and pressed Stop a second time.
- `store.ts`'s `stop()` gated both of its "stop the parent loop + interrupt the parent" branches on `!childFocused`, so whenever a child was focused it fell straight through to a bare `client.interrupt(runId)` targeting only the child's run id.
- This was the original, intentional scope of Task-088 ("Out of Scope: Stopping the entire agent loop from the child view") but CP-19/user expectation is that Stop always halts the whole live flow in one press, matching the guarantee CA-223 already established for the main-chat Stop button.
- Fix: `stop()` now always drives the parent-loop-stop / parent-interrupt path when the parent has a stoppable loop or is `workflow_step_auto`, regardless of which run is focused, and additionally best-effort-interrupts the *other* run (child when parent-focused-cascade fires, parent when the plain fallback fires) so both sides settle in one click. No backend change was needed — `stopAgentLoop` and `Interrupt` already cascade to every running child (CA-223); the bug was purely in the frontend's routing of which path to take.

### Current Ask

- User-reported: "khi bấm stop, nếu có child agent đang chạy -> nó chỉ stop child. Tôi phải bấm stop thêm 1 lần mới stop cả main agent." (Pressing Stop while a child agent is running only stops the child; a second press is needed to also stop the main agent.) Fix so Stop halts everything in one press.

### Key Decisions

- `D-1` Remove the `!childFocused &&` guard from both the loop-stop branch and the `workflow_step_auto` branch in `stop()` — these branches always target `parentRunId`, so letting them fire while a child is focused is what makes the parent (and its loop) actually stop from that view.
- `D-2` Keep a best-effort secondary `interrupt()` call for the "other" run in every branch, wrapped in try/catch, mirroring the existing "interrupt may 404 if no turn in flight" pattern already used elsewhere in this function — this covers the specific focused child/parent turn immediately rather than relying solely on the backend's own cascade.
- `D-3` Superseded Task-088's original out-of-scope note ("Stopping the entire agent loop from the child view... available on the orchestration board") — that scope decision is the confirmed root cause of this bug and is intentionally overridden here, not preserved.

### Constraints

- No backend (`interactive_service.go`) changes required — `Interrupt` and `stopAgentLoop` already cascade parent-to-children (CA-223); only the frontend `stop()` routing was wrong.
- Scoped to `store.ts`'s `stop()`; no changes to `ChatInput.tsx`'s Stop button wiring, `stopAgentLoop()` (the separate orchestration-board action), or any client/contract types.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/state/store.ts` `stop()` (line ~1204).
- `apps/desktop-flowpilot/src/state/store.test.ts` — updated `"stop keeps child-focused stop on the child run"` (encoded the bug) into `"stop from a child-focused view also stops the parent loop (BUG-247)"`; added a second case covering the tracked-loop path.
- `apps/local-runner/internal/runner/interactive_service.go` `stopAgentLoop`, `Interrupt` — unchanged; already cascade to children (CA-223).

## 1. Issue Summary

From a child agent's focused (read-only) chat view, clicking the Stop button (Task-088) interrupted only that child. The parent run — including any active agent loop driving further turns or re-spawns — kept running. The user had to click "Main" to return to the parent chat and press Stop again to actually halt the whole flow.

## 2. Parent Links

- task: [Task-088](../../08-Task/done/Task-088-Desktop-Child-Agent-Stop-Button.md) introduced the child-focused Stop button and explicitly scoped it to child-only, deferring "stop the entire loop" to the orchestration board.
- coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md).
- prior related fix: [CA-223](../../../change-audit/CA-223-parent-interrupt-cascades-to-child-agents.md) made the *main*-chat Stop button cascade to children; this bug is the mirror-image gap on the *child*-focused Stop button.

## 3. Environment and Reproduction

- environment: Desktop app, any provider, a chat with an actively running child agent (spawned via `spawn_agent` or a Flow Mode loop).
- reproduction steps:
  1. Start a chat/flow that spawns a child agent; wait until the child is `running`.
  2. Open the child agent's focused transcript view (child chat becomes read-only, Stop button appears next to Main per Task-088).
  3. Click Stop.
  4. Observe: the child stops (spinner/blocked state clears for the child), but the parent chat is still `running`/blocked.
  5. Click "Main" to return to the parent chat, then click Stop again — only now does the parent (and its loop) actually stop.
- frequency: deterministic, every time Stop is pressed from a child-focused view while the parent still has an active loop or in-flight turn.

## 4. Expected vs Actual

- expected: pressing Stop from the child-focused view stops the child **and** the parent (and its loop) in one action, exactly as pressing Stop from the main chat already did per CA-223.
- actual: only the child stopped; the parent required a separate Stop press from the main chat view.

## 5. Impact

- users affected: any user running multi-agent flows who inspects a child's transcript before wanting to stop everything.
- workflows affected: Flow Mode loops and ad-hoc `spawn_agent` chats.
- severity: medium — not data-lossy, but directly contradicts the "Stop halts the whole live flow" guarantee CA-223 established, and wastes provider turns/tokens on the still-running parent until the second click.

## 6. Root Cause

- confirmed cause: in `store.ts`'s `stop()`, both branches that call `client.stopAgentLoop(parentRunId)` / `client.interrupt(parentRunId)` were gated on `!childFocused`. When a child was focused, `childFocused` was `true`, so execution skipped both branches and fell through to the final `await client.interrupt(runId)`, where `runId` is the focused child's run id (per Task-088 T-1: `focusAgentRun` sets the store's `runId` to the child's id). The backend's `Interrupt(runID)` only cascades to children when `rs.parentRunID == ""` (i.e., called with a root run id) — calling it with the child's own id never reaches the parent.
- confirmed via code inspection of `store.ts` (the `!childFocused &&` guards) cross-referenced with the passing test `"stop keeps child-focused stop on the child run"` in `store.test.ts`, which asserted exactly this (now-fixed) behavior, and Task-088's own "Out of Scope" note documenting it as an intentional prior design choice.

## 7. Fix Strategy

- `F-1` Remove `!childFocused &&` from the loop-stop branch's condition (`parentRunId && client.stopAgentLoop && hasActiveParentAgentLoop(...)`) so it fires regardless of focus, and add a best-effort `client.interrupt(runId)` after the existing `client.interrupt(parentRunId)` when `childFocused` is true.
- `F-2` Remove `!childFocused &&` from the `chatMode === "workflow_step_auto"` branch the same way, with the same best-effort child interrupt appended.
- `F-3` In the final fallback (`await client.interrupt(runId)`, taken when neither of the above conditions apply), add a best-effort `client.interrupt(parentRunId)` when `childFocused && parentRunId !== runId`, so a child-focused Stop still reaches the parent even when the parent has no tracked loop.

## 8. Validation

- `V-1` `apps/desktop-flowpilot`: `tsc --noEmit` — clean.
- `V-2` Updated `store.test.ts`: `"stop from a child-focused view also stops the parent loop (BUG-247)"` (chatMode `workflow_step_auto`, no pre-seeded `agentGraphSnapshot`) and `"stop from a child-focused view with an active tracked loop stops parent and child (BUG-247)"` (chatMode `normal_chat`, active `agentGraphSnapshot.loopState.status: "running"`) — both assert `client.stopAgentLoop`/`client.interrupt` are called against `parent-1` before the redundant `interrupt:child-1`.
- `V-3` Full `store.test.ts` suite run via an isolated `tsc` + Node `--test` harness (scoped tsconfig + a `@/`/`@flowpilot/client-core` alias-resolution hook, since this repo's `test:phase1` script does not itself execute `store.test.ts`): 76 tests, 74 passed, 2 pre-existing environment-only failures (`localStorage is not defined` outside a DOM environment; one timing-sensitive abort test) — identical failure set confirmed present on the pre-fix baseline via `git stash`, so zero regressions introduced.
- `V-4` No Go changes were made; `go vet ./internal/runner/...` on `apps/local-runner` — no issues (confirms the backend cascade this fix now actually reaches was already correct per CA-223).

## 9. Regression Guard

- tests: `store.test.ts` — `"stop from a child-focused view also stops the parent loop (BUG-247)"`, `"stop from a child-focused view with an active tracked loop stops parent and child (BUG-247)"`.
- audit checks: `gitnexus_detect_changes()` was not run this session — GitNexus MCP tools were unavailable in this thread; see Constraints/Follow-Up below.

## 10. Follow-Up Document Updates

- [Task-088](../../08-Task/done/Task-088-Desktop-Child-Agent-Stop-Button.md)'s "Out of Scope" note ("Stopping the entire agent loop from the child view... available on the orchestration board") is superseded by this fix — left unedited as a historical record of the original (now-reversed) design decision rather than rewritten, consistent with BugFix documents being deltas, not replacements, of upstream truth.
- No SS/SD change needed — cascading Stop-to-everything was already the documented intent behind CA-223; this fix closes the one remaining gap rather than introducing a new rule.
