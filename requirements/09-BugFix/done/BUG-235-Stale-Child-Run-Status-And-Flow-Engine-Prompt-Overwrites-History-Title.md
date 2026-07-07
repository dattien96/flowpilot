# BUG-235: Stale Child Agent Run Status After Flow Done; Internal Flow-Engine Prompt Overwrites Chat-History Title

## Metadata

- Document ID: `BUG-235`
- Title: `Stale Child Agent Run Status After Flow Done; Internal Flow-Engine Prompt Overwrites Chat-History Title`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-03`
- Last Updated: `2026-07-03`
- Parent Documents: [BUG-234: Review-Loop Back-Edge Lifecycle](../done/BUG-234-Loop-Back-Lifecycle-Runaway-Advance-And-Cohort-Node-Settlement.md), [BUG-233: Flow Timeline Staleness And Blocked-Card Diagnostic Content](../done/BUG-233-Flow-Timeline-Staleness-And-Blocked-Card-Diagnostic-Content.md)
- Child Documents: `none`
- Related Documents: `none`
- Replaces: `none`
- Tags: `agent-flow-engine, review-loop, chat-history, agents-panel, desktop`

## AI Quick View

### Summary

Live testing after BUG-234 surfaced three more observations, all investigated (research-only pass) before fixing:

1. **Stale "running" reviewer after flow done** (fixed): a reviewer child agent could stay `running` in the Agents panel and as a leftover inline card in the main chat even after the flow completed cleanly (both reviewers approved, no hang). Root cause: `markFlowRunComplete` settled the flow's STEP timeline but never touched a child agent RUN's own status; separately, the desktop's `mergeAgentRunsById` let a stale, in-flight HTTP refresh response (captured before a child finished) overwrite an already-correct terminal status with a non-terminal one — and since a terminal child never fires another event, it stayed wrong forever.
2. **Chat-history title mis-set** (fixed): a flow run's history entry titled itself from whichever internal flow-engine prompt ran last ("[flow-engine] Agent results ready...") instead of the user's original request, because `lastPrompt` — the field the history list titles from — was overwritten unconditionally on every turn, including internal hub-reinvoke/coder-reentry turns.
3. **"Can't navigate back to a running flow after switching chats"** (investigated, NOT fixed this round): root-caused to `selectProject` resetting `runId`/`mainRunId` and `runHistory` being project-scoped, with no cross-project/cross-chat active-run tracking. This is a larger navigation/UX design question (e.g. an "Active runs" surface) rather than a contained bug; deferred — see `D-3`.
4. **"Main agent doing the review's own work"** (investigated, NOT a bug): the transcript shown was confirmed to be the reviewer's OWN turn, correctly isolated from the main timeline by `isEventForRun`'s run-id filter — not a leak. The leftover "reviewer · running" card next to it was symptom #1 above, which made it look concurrent. Synthesis running its own inspection tools instead of only synthesizing the join note is hub-model behavior (the auto-reinvoke prompt doesn't forbid tool use), not an engine defect.

### Current Ask

- Implemented: #1 and #2 (`D-1`, `D-2`), each with backend + desktop-side regression tests. #3 documented as a known follow-up, not implemented (`D-3`). #4 confirmed not a bug (`D-4`).

### Key Decisions

- `D-1` (#1) Two independent, complementary fixes rather than one: (a) backend `markFlowRunComplete` now force-settles any non-terminal, not-in-flight child run to `completed` when the flow reaches `done` (a defense-in-depth invariant: "flow done" implies no lingering child); (b) desktop `mergeAgentRunsById` now treats run status as monotonic — an incoming non-terminal status can never overwrite an existing terminal one, closing the actual race (a stale HTTP snapshot resolving after the correct SSE update).
- `D-2` (#2) `lastPrompt` is skipped for internal flow-engine-prefixed prompts ("[flow-engine]...") unless it's still empty, rather than adding a new persisted field — every internal turn (auto-reinvoke, coder re-entry, Continue-resume note) already carries that exact marker, so this is a minimal, low-risk fix with no schema/persistence changes.
- `D-3` (#3) NOT fixed this round. The root cause (project switch clears `runId`/`mainRunId`; history is project-scoped; no active-run tracking across projects/chats) implies a real feature (e.g. a cross-project "Active runs" list, or preserving/restoring the active run on project switch) rather than a one-line fix. Needs a design decision on the desired UX before implementation.
- `D-4` (#4) Confirmed not a bug via code inspection (`isEventForRun` correctly scopes timeline events by `workflowRunId`/`parentRunId`) — no fix needed. Noting here so it isn't mistakenly reopened.

### Open Questions

- `Q-1` (#3) What should "returning to an active flow after switching chats" look like — restore the exact prior run automatically on project reselect, or a persistent "Active runs" affordance independent of project/chat navigation? Needs product/UX input before implementation.

### Source Refs

- `apps/local-runner/internal/runner/flow_step_runtime.go` `markFlowRunComplete` — settled steps + run status but never child run status; new `reconcileChildRunsOnFlowDone`.
- `apps/desktop-flowpilot/src/state/store.ts` `mergeAgentRunsById` (now exported) — "incoming wins" with no monotonicity check; `refreshAgentRuns`'s own comment already acknowledged "this fetch is fire-and-forget and can land after a newer SSE agent-graph update" but the `_agentRunsLoadSeq` guard only protects against a newer REFRESH superseding an older one, not against racing the SSE channel.
- `apps/local-runner/internal/runner/interactive_service.go` `startTurn` — `rs.lastPrompt = truncateDisplayField(in.Prompt, 100)` unconditional on every turn.
- `apps/desktop-flowpilot/src/components/Navigator.tsx` — `runTitle(item.lastPrompt || item.lastMessage)`, the history title source.
- `apps/desktop-flowpilot/src/state/store.ts` `selectProject`/`resetRun` (#3) — clears `runId`/`mainRunId` on project switch; `runHistory` is project-scoped.
- `apps/desktop-flowpilot/src/state/store.ts` `isEventForRun` (#4) — confirmed correctly filters by `workflowRunId`/`parentRunId`.

## 1. Issue Summary

Four observations from live testing after BUG-234: a stale "running" child agent after a clean flow completion, a chat-history entry titled by internal engine text instead of the user's task, an inability to navigate back to a still-running flow after switching chats, and an apparent (but not real) leak of a reviewer's transcript into the main chat. The first two are genuine bugs, now fixed. The third is a real gap requiring a larger UX decision, documented but deferred. The fourth is not a bug.

## 2. Parent Links

Surfaced during live testing immediately after [BUG-234](../done/BUG-234-Loop-Back-Lifecycle-Runaway-Advance-And-Cohort-Node-Settlement.md) shipped.

## 3. Environment and Reproduction

- #1: built-in Review Loop, single round, both reviewers approve, flow reaches `done` cleanly (not a hang) — one reviewer's card still read "running" in the Agents panel and the main chat.
- #2: after a multi-round loop, the History list showed several entries titled "[flow-engine] Agent results ready. Re..." instead of the original task text.
- #3: started a flow, switched to a different chat/history item, then could not find a way back to the still-running flow's chat.
- #4: while a reviewer was running, the main chat area showed tool-call/thinking text and a "reviewer · running" card; confirmed via code reading (not a live repro) that this is the reviewer's own isolated transcript, not a leak.

## 4. Expected vs Actual

- #1 — Expected: once a flow is done, every agent-run entry it spawned reads a terminal status. Actual: a child could stay `running` indefinitely with no further event to correct it.
- #2 — Expected: a flow run's history title reflects what the user asked for. Actual: it reflected the last internal engine prompt that happened to run.
- #3 — Expected: a way to return to an active flow run after navigating away. Actual: no such path once the project/chat context changes.

## 5. Impact

- #1: confusing status display; a user might think a flow is still consuming provider budget when it isn't.
- #2: history becomes hard to navigate — multiple runs indistinguishable by title.
- #3: a user can lose track of / be unable to resume a long-running flow.

## 6. Root Cause

- #1: two independent gaps compounded — backend `markFlowRunComplete` never reconciled child run statuses on flow completion, and the desktop's `mergeAgentRunsById` had no protection against a stale (captured-before-completion) HTTP snapshot resolving after the correct SSE-driven terminal update and reverting it, with no subsequent event to self-correct.
- #2: `rs.lastPrompt` — the exact field the history list titles from — was overwritten unconditionally on every `startTurn` call, including the internal turns the flow engine schedules for itself (hub auto-reinvoke, coder re-entry, Continue-resume), all of which are prefixed `"[flow-engine]"`.
- #3: `selectProject` unconditionally clears `runId`/`mainRunId` via `resetRun`, and `runHistory` is scoped per-project with no independent tracking of "runs currently active regardless of which project/chat is open."
- #4: not a defect — `isEventForRun` correctly scopes every timeline event by its originating run id before it's ever applied to `s.timeline`.

## 7. Fix Strategy

- #1: (a) backend — `reconcileChildRunsOnFlowDone`, called from `markFlowRunComplete`, force-settles any child that is not `turnInFlight` and still `running`/`waiting_approval`/`waiting_question` to `completed`, re-upserts its summary, and emits one fresh `agent_graph_updated` so the desktop clears it immediately; (b) desktop — `mergeAgentRunsById` treats status as monotonic: an incoming non-terminal status is dropped when the existing entry for that run id is already terminal.
- #2: skip the `lastPrompt` overwrite in `startTurn` when the incoming prompt is internal (`"[flow-engine]"`-prefixed), unless `lastPrompt` is still unset.
- #3: deferred — no fix implemented; needs a UX decision (`Q-1`) first.
- #4: no fix — confirmed correct behavior.

## 8. Validation

- `go build ./...` / `go vet ./internal/runner/...` — clean. Desktop `npm run typecheck` / `npm run build` — clean.
- `go test ./internal/runner/...` — 13 pre-existing environment-specific failures only (identical baseline through BUG-233/234); no new failures.
- New tests, all green:
  - `TestMarkFlowRunCompleteSettlesLingeringChildAgentRun` — a lingering "running" child (not in flight) is force-settled to `completed`, with its summary updated, when `markFlowRunComplete` runs.
  - `TestStartTurnPreservesOriginalPromptOverInternalFlowEngineTurns` — an internal hub-reinvoke turn does not overwrite the original user prompt in `lastPrompt`.
  - `mergeAgentRunsById never lets a stale non-terminal snapshot revert an already-terminal run` / `...still applies a genuinely fresh non-terminal update for a never-terminal run` / `...applies a terminal incoming status over an existing terminal one` (`store.test.ts`) — the monotonicity rule and its two edge cases (non-terminal→non-terminal still updates; terminal→terminal status corrections still land, e.g. completed→failed).

## 9. Regression Guard

- The four tests listed under Validation above, covering both the backend reconciliation and the desktop monotonic-merge rule, plus the `lastPrompt` preservation rule.

## 10. Follow-Up Document Updates

- None required for #1/#2 (self-contained to `apps/local-runner` + `apps/desktop-flowpilot`, no contract/schema changes). #3 remains open; a future doc should capture the chosen UX design before implementation.

## 11. Definition of Done

- `[x]` **DOD-1 — #1 backend fix.** `reconcileChildRunsOnFlowDone` settles lingering non-terminal, not-in-flight children to `completed` on flow `done`.
- `[x]` **DOD-2 — #1 desktop fix.** `mergeAgentRunsById` never lets a non-terminal incoming status overwrite an existing terminal one.
- `[x]` **DOD-3 — #1 regression tests.** Backend (`TestMarkFlowRunCompleteSettlesLingeringChildAgentRun`) and desktop (3 `mergeAgentRunsById` tests) both green.
- `[x]` **DOD-4 — #2 fix.** `startTurn` no longer overwrites `lastPrompt` with an internal `"[flow-engine]"`-prefixed turn's prompt.
- `[x]` **DOD-5 — #2 regression test.** `TestStartTurnPreservesOriginalPromptOverInternalFlowEngineTurns` green.
- `[x]` **DOD-6 — #3/#4 documented.** #3's root cause and open UX question recorded (`D-3`, `Q-1`); #4 confirmed not a bug (`D-4`).
- `[x]` **DOD-7 — Full regression green.** `go build`/`vet`/`test` at the established baseline; desktop `typecheck`/`build` clean.
- `[x]` **DOD-DOC — Docs.** This doc in `done/`; `change-audit/CA-237-*` written.

## 12. Code Change Plan (per DoD item)

### DOD-1 / DOD-3 — backend child-run reconciliation
- `flow_step_runtime.go` `markFlowRunComplete`: added a call to new `reconcileChildRunsOnFlowDone(parentRunID)` after step/run-status settlement.
- `flow_step_runtime.go`: new `reconcileChildRunsOnFlowDone` — locks `s.mu`, iterates `s.agentOrchestrator.listChildren(parentRunID)`, skips `nil`/`turnInFlight` children, force-sets `running`/`waiting_approval`/`waiting_question` children to `completed` (both `interactiveRun.status`/`agentStatus` and the orchestrator's `AgentRunSummary`), then emits one `agent_graph_updated` if anything changed.
- Test: `TestMarkFlowRunCompleteSettlesLingeringChildAgentRun` (`flow_step_runtime_test.go`) — directly constructs a lingering child run (bypassing the async turn machinery, whose retry timing is orthogonal to what's being proven) and asserts both the run status and summary settle to `completed`.

### DOD-2 / DOD-3 — desktop monotonic merge
- `store.ts` `mergeAgentRunsById` (now exported for direct testing): when merging `incoming` into `existing`, skip an incoming entry whose status is non-terminal if the existing entry for that run id is already terminal (`isTerminalRunStatus`, already defined, previously unused).
- Tests (`store.test.ts`): three new cases covering the revert-prevention rule and its two edge cases (still-updates for a never-terminal run; terminal→terminal corrections still land).

### DOD-4 / DOD-5 — history-title fix
- `interactive_service.go` `startTurn`: `rs.lastPrompt` update guarded by `!strings.HasPrefix(strings.TrimSpace(in.Prompt), "[flow-engine]") || rs.lastPrompt == ""`.
- Test: `TestStartTurnPreservesOriginalPromptOverInternalFlowEngineTurns` (`interactive_service_test.go`) — drives a real user turn then a real hub-reinvoke turn (`autoReinvokePromptText()`) and asserts `lastPrompt` is unchanged.

### DOD-6 — #3/#4 documentation only
- No code. `D-3`/`Q-1` record the root cause and the open UX question; `D-4` records the not-a-bug confirmation.
