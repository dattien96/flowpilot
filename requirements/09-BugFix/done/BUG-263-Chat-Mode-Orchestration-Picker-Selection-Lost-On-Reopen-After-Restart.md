# BUG-263: Chat Mode Orchestration Picker Selection Lost On Reopen After Restart

## Metadata

- Document ID: `BUG-263`
- Title: `Chat Mode Orchestration Picker Selection Lost On Reopen After Restart`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [CP-36: Agent Review Loop And Main Hub Orchestration](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) (Scenario 5, Scenario 11), [BUG-170: Workflow Flow Mode History Unresumable After Server Restart](./BUG-170-Workflow-Flow-Mode-History-Unresumable-After-Server-Restart.md) (established the chatMode/launchMode restore-on-reopen pattern this bug's fix mirrors for chatStartMode/flowRef)
- Child Documents: `none`
- Related Documents: [BUG-261: Chat-Mode Explicit FlowRef Resolve Failure Silently Suppresses Hub Turn Forever](./BUG-261-Chat-Mode-Explicit-FlowRef-Resolve-Failure-Silently-Suppresses-Hub-Turn-Forever.md) (same live testing session, same explicit chat flowRef code path), [CA-261](../../../change-audit/CA-261-restore-chat-orchestration-picker-selection-on-reopen.md)
- Replaces: `none`
- Tags: `agent-flow-engine, chat-mode, ui, restart, resume, regression`

## AI Quick View

### Summary

- Found live during CP-36 Scenario 11 testing (`run-11120`): the user started a Chat Mode run via the Bug sub-mode's Built-in orchestration picker (Review Loop), killed and restarted the local runner (Scenario 5's repro), then reopened the run — the run's own content and flow state resumed correctly, but the Chat Intent panel showed **"Normal"** selected (and locked), not **"Bug"** with Review Loop, even though the run was genuinely still a flow-engine-driven Review Loop run underneath.
- Root cause: the explicit chat `subMode`/`flowRef` a run was started with was never stored anywhere beyond the transient `TurnInput` fields inside `startTurn` — not on the in-memory run, not in persisted session state, not in the run-history API. `openHistoryRun`'s reopen logic (the same code path BUG-170 already fixed for `chatMode`/`launchMode`) therefore had nothing to restore `chatStartMode`/`flowRef` from, so it silently kept the store's default value of `"normal"`.
- This is the same class of gap BUG-170 already fixed once — for `chatMode`/`launchMode` — just never extended to the newer (CP-42/Task-177) Chat-Mode picker fields introduced afterward.

### Current Ask

- Reopening a run started via Chat Mode's Bug sub-mode Built-in orchestration picker must restore the Chat Intent panel to the exact selection (Bug tab, Review Loop) it was started with, not silently default to Normal.

### Key Decisions

- `V-1` Persist the selection at the source: `startTurn` now stores `subMode`/`flowRef` on the run itself (`rs.chatSubMode`/`rs.chatFlowRef`) alongside the existing `flowEngineDriven` flag, in the same first-turn branch.
- `V-2` Round-trip through every layer already used for the analogous flow-topology fields (`ActiveFlowEdges`/`ActiveFlowNodes`, BUG-NOTE-CP42 #16): `ProviderSessionState` → NDJSON record (`local_file_session_store.go`) → in-memory reconstruction (`reconstructRun`) → run-history API (`runHistoryItem`) → desktop `RunHistoryItem` contract → `openHistoryRun`'s restore `set(...)` call. No new persistence mechanism invented; this is the identical shape BUG-170 established.
- `V-3` `chatStartMode` is derived from `subMode === "bug"` rather than stored as its own separate `ChatStartMode`-typed value end to end, since the backend only ever knows about the runner-facing `"bug"` sub-mode key (the desktop's own `ChatStartMode` enum, `"normal" | "task" | "bugfix"`, is a client-side concept with no direct backend equivalent for `"task"` today).

### Constraints

- Does not change `flowEngineDriven` itself, which still isn't restored across a restart from persisted state (a separate, pre-existing gap noted but not fixed here — `isFlowEngineDriven` reads live-only `rs.flowEngineDriven`, and other restart-survival fixes in this area (BUG-256/257/260) already work off `activeFlowNodes`/`activeFlowEdges` instead, so this doesn't block anything the picker-selection fix needs).
- Only the explicit chat flowRef path is covered; the Flow-Mode workflow-picker path already has its own correct chatMode/launchMode restore via BUG-170 and needs no change.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go`: `interactiveRun.chatSubMode`/`chatFlowRef` fields (~line 183+), set in `startTurn`'s explicit-flowRef branch (~line 3312), read in `sessionStateOf` (~line 1577).
- `apps/local-runner/internal/runner/workflow_store.go`: `ProviderSessionState.ChatSubMode`/`ChatFlowRef` (~line 116).
- `apps/local-runner/internal/runner/local_file_session_store.go`: `ndjsonSessionRecord.ChatSubMode`/`ChatFlowRef` and both `sessionStateFromRecord`/`sessionRecordFrom` mappers.
- `apps/local-runner/internal/runner/interactive_resume.go`: `reconstructRun` (~line 689).
- `apps/local-runner/internal/runner/interactive_handlers.go`: `runHistoryItem.SubMode`/`FlowRef` and `projectRunHistory`'s two population sites (in-memory `rs` list and persisted-session list).
- `apps/desktop-flowpilot/src/types/contract.ts`: `RunHistoryItem.subMode`/`flowRef`.
- `apps/desktop-flowpilot/src/state/store.ts`: `openHistoryRun`'s restore `set(...)` (~line 1506+).
- New tests: `TestLocalFileSessionStoreChatOrchestrationSelectionRoundTrip` (`local_file_session_store_test.go`), `TestChatModeRunHistoryExposesOrchestrationPickerSelection` (`flow_executor_test.go`), `openHistoryRun restores the Chat Mode orchestration picker selection (BUG-263)` + the plain-chat negative case (`store.test.ts`).

## 1. Issue Summary

A live CP-36 Scenario 11 test (`run-11120`) reopened a Chat Mode Review Loop run after a runner restart. The run's transcript, flow topology, and agents all resumed correctly, but the Chat Intent panel's Bug tab / Built-in orchestration selection reset to "Normal", contradicting the panel's own "Locked after the first message" copy, which implies the original selection should persist and simply become non-interactive, not silently change.

## 2. Parent Links

- impacted coding plan: [CP-36](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), Scenario 5 (restart repro) and Scenario 11 (Chat-Mode picker deep verification) — found while running Scenario 11's additional Chat-Mode parity checks.
- impacted design precedent: BUG-170's chatMode/launchMode restore-on-reopen fix is the exact template this bug's fix follows for the newer CP-42/Task-177 picker fields.

## 3. Environment and Reproduction

- environment: Desktop app + local-runner, Chat Mode Bug sub-mode → Built-in orchestration → Review Loop.
- reproduction steps:
  1. Start a Review Loop run via the Bug tab's Built-in orchestration picker.
  2. Kill and restart the local runner process (Scenario 5's repro).
  3. Reopen the run (from history or the still-visible active chat).
  4. Observe: the Chat Intent panel shows "Normal" selected instead of "Bug" / Review Loop.
- frequency: deterministic for any explicit-chat-flowRef run reopened after the in-memory run state is lost (a runner restart, or any reopen-from-persisted-history path).

## 4. Expected vs Actual

- expected: reopening a Chat Mode Review Loop run restores the Chat Intent panel to show Bug / Review Loop as the (locked) selection, matching what the run was actually started with.
- actual: the panel showed "Normal" — a silently wrong restoration, not an error, so nothing alerted the user that the display was inaccurate (the flow itself kept working correctly underneath; this was a display-only regression).

## 5. Impact

- users affected: anyone reopening a Chat Mode Review Loop run after a runner restart or from run history in a fresh session.
- workflows affected: display only — `chatStartMode`/`flowRef` are read defensively (`isFirstChatTurn && ...`) so they don't affect an already-started run's actual behavior; a NEW turn on the reopened run would not accidentally re-trigger flow start logic incorrectly. The impact is purely that the UI misrepresents which orchestration is active.
- severity: medium — confusing/misleading UI state, no data loss or functional regression to the flow itself.

## 6. Root Cause

- confirmed cause: `subMode`/`flowRef` existed only as transient `TurnInput` fields consumed once inside `startTurn`'s `if flowRef := strings.TrimSpace(in.FlowRef); flowRef != "" { ... }` branch (`interactive_service.go`) — never assigned to any field on `rs` (the in-memory run), never included in `sessionStateOf`/`ProviderSessionState`, and therefore never in the NDJSON persistence or the run-history API response. `openHistoryRun` (`store.ts`) restores `chatMode`/`launchMode` from `historyItem.runKind`/`workflowId` (BUG-170) but had no equivalent source data for `chatStartMode`/`flowRef`, so those two fields kept whatever the store's default was (`"normal"`/`undefined`).
- evidence: live screenshot of `run-11120`'s Chat Intent panel post-restart showing "Normal" selected and locked, for a run confirmed (via the same session's other checks) to still be a genuine flow-engine-driven Review Loop run.

## 7. Fix Strategy

- `F-1` `interactive_service.go`: added `chatSubMode`/`chatFlowRef` fields to `interactiveRun`, set them in `startTurn`'s existing first-turn flowRef branch, and included them in `sessionStateOf`.
- `F-2` `workflow_store.go`: added `ChatSubMode`/`ChatFlowRef` to `ProviderSessionState`.
- `F-3` `local_file_session_store.go`: added the corresponding NDJSON fields and wired both directions of the record↔state mapping.
- `F-4` `interactive_resume.go`: `reconstructRun` now restores `chatSubMode`/`chatFlowRef` from the persisted state when rebuilding an in-memory run after a restart.
- `F-5` `interactive_handlers.go`: `runHistoryItem` exposes `SubMode`/`FlowRef`; `projectRunHistory` populates them from both the in-memory `rs` list and the persisted-session list (the exact branch this bug's live repro exercised, per BUG-060 F-1's own comment about restart survival).
- `F-6` `contract.ts`: `RunHistoryItem.subMode`/`flowRef` added.
- `F-7` `store.ts`: `openHistoryRun` now derives `chatStartMode` from `historyItem.subMode === "bug" ? "bugfix" : "normal"` and restores `flowRef` alongside the existing `chatMode`/`launchMode` restoration, in the same `set(...)` call.

## 8. Validation

- `V-1` `go build ./...` — clean. `go vet ./internal/runner/` — clean. `npm --prefix apps/desktop-flowpilot run typecheck` — clean.
- `V-2` New test `TestLocalFileSessionStoreChatOrchestrationSelectionRoundTrip` (`local_file_session_store_test.go`): writes a session with `ChatSubMode`/`ChatFlowRef` set, reloads the store from a fresh instance pointed at the same directory, confirms both fields survive. Passes.
- `V-3` New test `TestChatModeRunHistoryExposesOrchestrationPickerSelection` (`flow_executor_test.go`): drives the real HTTP `handleStartTurn` route with `subMode=bug`/`flowRef=review-loop`, then asserts `svc.projectRunHistory("proj")` returns the run with `SubMode`/`FlowRef` populated. Passes.
- `V-4` New tests in `store.test.ts`: `openHistoryRun restores the Chat Mode orchestration picker selection (BUG-263)` (asserts `chatStartMode="bugfix"`, `flowRef` set for a history item with `subMode="bug"`) and a negative case for a plain chat run (asserts `chatStartMode` stays `"normal"`, `flowRef` stays `undefined`). Verified via `npm run typecheck` only — direct `tsx --test` execution of `store.test.ts` is blocked by this repo's pre-existing unresolved `@/` Vite-alias limitation for frontend test files run outside the Vite toolchain (the same constraint noted in CA-251 for `AgentsPanel.test.ts`); not a gap introduced by this fix.
- `V-5` `go test ./internal/runner/... -count=1`: 1147 passed (2 more than the pre-fix baseline of 1145, from the 2 new Go tests), 15 failed (identical pre-existing, environment-specific failures — missing Codex CLI, Windows path assertions, skill-precedence tests needing real files — same names as BUG-261/BUG-262's own validation passes), 14 skipped.
- `V-6` GitNexus MCP tools were unavailable in this thread; proceeded via direct code inspection, tracing every read/write site of `ActiveFlowEdges`/`ActiveFlowNodes` (the precedent field pair) to find every layer `ChatSubMode`/`ChatFlowRef` needed to be added to.
- `V-7` Not performed: a live restart-and-reopen re-test against the rebuilt binary (the original live repro's evidence was a screenshot from before this fix; a follow-up live pass would confirm the picker now shows "Bug" / Review Loop after reopening `run-11120`-equivalent runs).

## 9. Regression Guard

- tests: `TestLocalFileSessionStoreChatOrchestrationSelectionRoundTrip`, `TestChatModeRunHistoryExposesOrchestrationPickerSelection` (Go); `openHistoryRun restores the Chat Mode orchestration picker selection (BUG-263)` and its plain-chat negative case (`store.test.ts`).
- audit checks: `gitnexus_detect_changes()` was not run — GitNexus MCP tools were unavailable in this thread; see `V-6`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none — this restores CP-36's own already-stated intent (a locked, accurate Chat Intent panel selection) rather than changing any acceptance criteria.
- notes left unchanged on purpose: `flowEngineDriven`'s own lack of restart-persistence (noted in Constraints) is a separate, pre-existing gap not exercised by any currently-failing scenario and left untouched here.
