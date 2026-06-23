# CA-117: Fix Tool-Spawn Wait-False Result Delivery

## Scope

- Parent provider-conversation result delivery for background sub-agents
- Tool vs UI spawn symmetry for both wait modes
- Reuse of the BUG-122 `pendingAgentContext` injection buffer

## Completed

- Added `interactiveRun.waitForResult`, stamped from `SpawnAgentInput.Wait` in `spawnChildRun`.
- Changed the child completion and failure injection conditions from `uiInitiated` to `uiInitiated || !waitForResult`.
- Tool `wait=false` children now inject their result into the parent's next provider turn; tool `wait=true` children remain excluded (already returned synchronously).
- Repurposed the prior `TestToolSpawnDoesNotInjectParentContext` into `TestToolSpawnWaitTrueDoesNotInjectParentContext` and added `TestToolSpawnWaitFalseInjectsResult`.

## GitNexus Impact

- GitNexus index was reported stale during this change; impact was assessed by local inspection.
- `emitLocked`: change is additive to an existing condition; only the child-completion/failure context-injection branch is affected.
- `spawnChildRun`: one additional field assignment at child stamping; no control-flow change.
- No HIGH or CRITICAL behavior change: the spawn lifecycle, waiters, and graph/bus events are untouched.

## Verification

- `TestToolSpawnWaitFalseInjectsResult` — pass.
- `TestToolSpawnWaitTrueDoesNotInjectParentContext` — pass.
- `TestUISpawnInjectsContextIntoParentProviderTurn` — pass.
- `go build ./internal/runner/...` — pass.
- Broader spawn/agent suite green except pre-existing, unrelated environment failures (skills-merge provider-home tests; a Windows-only rename failure in the separate BUG-124 migration test, flagged separately).

## Residual Notes

- Only the child's final result is shared; the full child transcript is intentionally never exposed to the parent (multi-agent isolation).
- Injection rides the existing BUG-122 `pendingAgentContext` buffer and its `sessions.ndjson` persistence; no new mechanism added.
