# CA-226: Hub Synthesis Fallback Escalation

## Summary

Implemented a per-turn guard to detect when the hub synthesis turn completes without calling `submit_review_outcome` (e.g. if the model responds in prose only). In this case, the runner automatically transitions the parent run's loop state to `blocked` (escalated) and marks the inline synthesis step as `FAILED` on the timeline, preventing synthesis nodes from hanging in a permanent `RUNNING` state. Also fixed the workflow-handoff startup bug where `startTurn` launched the flow executor but never emitted a terminal event for the synthetic parent turn, which left the main chat stuck on `Thinking...`, kept Step 4 in `running`, and prevented later spawned-agent annotations from appearing. Added desktop-side reconciliation so live `agent_graph_updated` events refresh both step runtime and agent summaries, matching the manual focus-switch refresh path that revealed completed reviewers. Stop now treats an active parent loop as a loop run even when launched from normal chat + `flowRef`, so the chat Stop button stops the loop and interrupts the parent turn. Fixed a remaining synthesis hang where a note-bearing cohort join could race with an already scheduled/active empty hub reinvoke; the joined note is now preserved in `pendingAgentContext` instead of being dropped by the single-flight guard. Excluded cohort member completions from triggering individual hub turns in `releaseDependentAgents`, strengthened auto-reinvocation instructions, and fixed a Flow Mode hook-order crash in the Agents panel.

## What Changed

- `apps/local-runner/internal/runner/interactive_service.go`:
  - Added `lastFlowControlTurnID` field to `interactiveRun` struct.
  - Set `lastFlowControlTurnID = rs.currentTurnID` inside `applyFlowControl`.
  - Added a fallback guard inside `runTurn` (right after `finishTurn`) to call `applyFlowControl(..., "escalate")` and mark the hub node `FAILED` if the synthesis turn completes without calling the control tool.
  - Prevented cohort child completions from prematurely triggering `maybeAutoReinvokeHub` in `releaseDependentAgents`.
  - Updated fallback prompt string in `autoReinvokePromptText` to enforce calling `submit_review_outcome`.
  - In the `flowStartOnly` handoff path, emitted a synthetic `turn_completed` event, persisted the run snapshot, and cleared turn-in-flight state before returning so the initial parent turn always settles after launching the flow executor.
  - Preserved note-bearing `maybeAutoReinvokeHubWithNote` calls when the hub single-flight guard is already closed, so a scheduled/current synthesis turn still receives the joined cohort note instead of seeing only "Read the joined result note above" with no note.
- `apps/local-runner/internal/agentpack/flow-pack/prompts/auto-reinvoke.md`:
  - Strengthened prompt instructions to explicitly demand `submit_review_outcome` call and forbid prose-only responses.
- `apps/local-runner/internal/runner/interactive_service_e2e_test.go`:
  - Added e2e regression test `TestE2EReviewLoopSynthesisFallbackEscalates` verifying synthesis fallback correctly blocks the loop, fails the step, and does not loop endlessly.
- `apps/local-runner/internal/runner/interactive_service_test.go`:
  - Added regression tests proving note-bearing hub reinvokes preserve the joined note when a hub turn is already in-flight or already scheduled.
- `apps/local-runner/internal/runner/flow_executor_test.go`:
  - Added regression test `TestStartTurnWithFlowRefEmitsSyntheticTurnCompletedForHubHandoff` covering the flow-handoff startup path so parent turns do not remain in-flight forever after the executor starts.
- `apps/desktop-flowpilot/src/state/store.ts`:
  - Derived top-level run status from later orchestration graph snapshots so the UI stays `running` while the flow is still active even though the synthetic first turn now completes immediately.
  - Updated graph snapshot application to update `agentRuns` immediately from returned snapshots.
  - Reconciled agent summaries via `refreshAgentRuns()` whenever live `agent_graph_updated` events arrive, so stale running reviewer cards converge without switching focus.
  - Updated `stop()` to stop the active parent agent loop outside `workflow_step_auto` when a normal-chat `flowRef` run has an active loop snapshot, then interrupt the parent turn and clear the thinking row.
- `apps/desktop-flowpilot/src/state/store.test.ts`:
  - Added regression test covering the workflow handoff handshake: the initial turn settles, the orchestration stream keeps the run active, and later reviewer spawn annotations still appear in the parent timeline.
  - Added regression test proving stale reviewer statuses refresh to completed without switching focus.
  - Added regression test proving Stop handles normal-chat `flowRef` runs by stopping the loop and interrupting the parent turn.
- `apps/desktop-flowpilot/src/components/AgentsPanel.tsx`:
  - Moved the Flow Mode "no workflow/step selected" null return below all hooks so switching into Flow Mode does not render fewer hooks than the previous render.

## Verification

- `go test ./internal/runner -run 'TestE2EReviewLoop(ApprovedPath|SynthesisFallbackEscalates)|TestReviewOutcomeToolOfferedOnlyOnHubSynthesisTurn|TestSubmitFlowControlRejectsCohortMemberButAllowsHub'`
- `go test ./internal/runner -run 'TestE2EReviewLoop|TestFlow|TestReviewOutcome|TestSubmitFlowControl|TestCodexAdapterSubmitReviewOutcome'`
- `go test ./internal/runner -run 'TestAutoReinvokeHub(WithNotePreservesNoteWhenTurnInFlight|WithNotePreservesNoteWhenReinvokeAlreadyScheduled|NoPendingContextNoDefer|DeferredWhenCoderCompletesInFlight|SingleFlightConcurrent)|TestE2EReviewLoopSynthesisFallbackEscalates|TestE2EParallelCodingCohortReinvokesHub'`
- `go test ./internal/runner`
- `npm run typecheck` in `apps/desktop-flowpilot`
- `npx tsx --test --test-name-pattern 'workflow handoff turn settles and orchestration stream keeps the run active|sendPrompt applies live agent graph and bus SSE updates|sendPrompt keeps the parent orchestration stream alive after the turn completes' src/state/store.test.ts` in `apps/desktop-flowpilot`
- `npx tsx --test --test-name-pattern 'orchestration graph update refreshes stale agent run statuses without switching focus|stop uses loop stop plus parent interrupt for normal chat flowRef runs|stop uses loop stop plus parent interrupt for the main flow run|workflow handoff turn settles and orchestration stream keeps the run active' src/state/store.test.ts` in `apps/desktop-flowpilot`
- `npm run build` in `apps/desktop-flowpilot`

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-42
change_type: bugfix
summary: Add synthesis fallback escalation, settle flow handoff turns, refresh live agent status, and fix Flow Mode hook-order crash
# --->8---
