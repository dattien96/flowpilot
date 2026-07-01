# CA-162: BUG-NOTE-CP42 P1 Fixes — Hub Wait-Notice Race and Flow-Topology Race

## Scope

Verified and fixed two P1 issues from a Codex-sourced review (`requirements/09-BugFix/todo/BUG-NOTE-CP42.md`), both real races introduced by the CP-42 generic flow executor.

## BUG#1 — hub wait-notice lost to a goroutine race

`startTurn` dispatched `startResolvedFlow` asynchronously (`go s.startResolvedFlow(...)`) and then, still inside the same synchronous call, drained `rs.pendingAgentContext` into `capturedCtx` for the turn about to start. `notifyHubFlowStarted` (called from inside the async `startResolvedFlow`) appends the "an agent is already working, don't also code this yourself" notice to that same field — but by the time its goroutine runs, the drain has almost always already happened, so the hub's own first turn never saw the notice.

**Fix**: `startTurn` now resolves the wait notice (`prompts/flow-start-wait.md`, falling back to the same hardcoded string `notifyHubFlowStarted` uses) and prepends it synchronously to `in.Prompt` before dispatching the goroutine — see `interactive_service.go` around the `flowRef := strings.TrimSpace(in.FlowRef)` block. The goroutine still receives the original, unmodified prompt for the spawned child. `notifyHubFlowStarted`/`flow_executor.go` is unchanged; it remains correct for any *later* turn context (e.g. a flow started mid-conversation, not on `turnCount == 0`).

## BUG#4 — activeFlowEdges set after the entry child was already spawned

`startResolvedFlow` spawned the flow's entry node(s) via `spawnChildRun`, which starts the child's own first turn asynchronously before returning, and only *after* the full spawn loop finished did the old code set `rs.activeFlowEdges`/`rs.activeFlowNodes`. A fast-completing child (trivially reproducible with a fake/test adapter, plausible with a fast real provider) could complete and trigger `tryAdvanceFlowFromNode` before that assignment landed, silently falling back to the legacy note+reinvoke-hub path instead of deterministically auto-spawning the reviewer cohort.

**Fix**: the `rs.activeFlowEdges`/`rs.activeFlowNodes` assignment now happens immediately after resolving the flow definition, before the entry-node spawn loop runs at all — eliminating the timing dependency rather than narrowing the window.

## Verification

- New test `TestStartTurnWithFlowRefPrependsWaitNoticeToHubsOwnFirstTurn` (`flow_executor_test.go`): drives the real `startTurn` wiring path with a fake adapter that records the parent run's own provider-observed prompt, and asserts it contains both the wait notice and the original user prompt.
- Existing `TestCoderCompletionAutoSpawnsReviewerCohort` continues to pass and exercises the corrected BUG#4 ordering (its fake adapter completes the coder's turn essentially immediately — the exact fast-completion scenario the bug describes).
- `go build ./...` clean; full `go test ./...` run: 1189 passed, 16 failed (all pre-existing/environmental — Windows home-directory paths, missing local `codex` CLI, fixture assumptions — none in a file touched by this change), 14 skipped, matching the pre-existing baseline noted in CA-161.

## Still open

Remaining P1/P2/P3 items in `BUG-NOTE-CP42.md` (#2, #9, #10, #13, #18, #23, #24, and the P2/P3 list) are not addressed by this note — tracked separately, to be verified and fixed incrementally.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: fix hub wait-notice goroutine race (BUG#1) and activeFlowEdges-set-after-spawn race (BUG#4) from the BUG-NOTE-CP42 review
# --->8---
