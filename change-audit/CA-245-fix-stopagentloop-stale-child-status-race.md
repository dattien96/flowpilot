# CA-245: Fix stopAgentLoop Stale-Child-Status Race Behind Permanent Stop Hang

## Summary

Fixed a race that left the main run permanently stuck "running" after Stop, surfaced immediately after CA-244's fix started actually driving the parent-loop-stop path from a child-focused view. `stopAgentLoop`'s `turnCancel()` only signals cancellation — a child's own `status` flips to terminal asynchronously inside `finishTurn`, once its turn goroutine observes `ctx.Done()`. `stopAgentLoop` built its returned snapshot synchronously in the same instant, so it could (reliably, per a new deterministic test) report a just-cancelled child as still "running" — and since a cancelled child's `finishTurn` never re-emits `agent_graph_updated` for the parent, no later event ever corrected it, so a second Stop press did nothing either. Fixed on both sides: the desktop now treats a `"stopped"` loop as authoritative over any stale child status, and the backend now eagerly writes the cancelled status into both `interactiveRun.status` and `AgentOrchestrator`'s cached summary before building the snapshot it returns.

## What Changed

- `apps/desktop-flowpilot/src/state/store.ts`: `deriveOrchestrationRunStatus` — check `loopState.status === "stopped"` first and return `"cancelled"` immediately, ahead of the existing child-status precedence checks.
- `apps/local-runner/internal/runner/interactive_service.go`: `stopAgentLoop` — eagerly set `RunStatusCancelled` on the parent and every child whose `turnCancel()` it fires, on both `interactiveRun.status` and `AgentOrchestrator`'s cached summary (via `currentSummary`/`upsertSummary`), before building the returned `AgentGraphSnapshot`.
- `apps/desktop-flowpilot/src/state/store.test.ts`: added `"deriveOrchestrationRunStatus: a stopped loop wins even if a child's status snapshot is stale-running (BUG-248)"`.
- `apps/local-runner/internal/runner/interactive_service_test.go`: added `TestStopAgentLoopSnapshotReportsChildCancelledSynchronously`, using a fake adapter that blocks past `ctx.Done()` so `finishTurn` provably has not run when the snapshot is inspected.

## Verification

- `go build ./...` clean; `go test ./internal/runner/...` — 4 pre-existing environment-only failures (Windows path-quoting, provider-home skill precedence) confirmed identical on the pre-fix baseline, no regressions.
- `apps/desktop-flowpilot`: `tsc --noEmit` clean; `store.test.ts` full suite (77 tests) via an isolated harness — 75 passed, same 2 pre-existing environment-only failures as the pre-BUG-247 baseline.

# ---8<--- flowpilot:change-ledger
feature_key: agent-spawn
source_doc_id: BUG-248
change_type: bugfix
summary: Fix a race in stopAgentLoop where a cancelled child's stale "running" status could permanently stick the main run's derived status, making Stop appear to hang
# --->8---
