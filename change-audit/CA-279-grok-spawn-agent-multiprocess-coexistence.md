# CA-279: Grok Spawn Agent Multiprocess Coexistence

## Scope

Fixes Task-209 DOD-2 live regression: spawning a Grok child agent during a parent turn no longer tears down the parent's in-flight `grok agent stdio` process with `grok agent process torn down`. Child agents also inherit the parent's current per-turn model/reasoning-effort instead of the stale createRun baseline.

## Root Cause

- `Runner.grokProcess` was a singleton keyed only by account scope. `ensureGrokProcess` closed the live process on any model/effort/always-approve mismatch. A child turn with a different launch tuple killed the parent's dispatcher mid-`session/prompt`.
- `runTurn` resolved per-turn `TurnInput.Model`/`ReasoningEffort` for the adapter but did not persist them onto `interactiveRun`, so `spawnChildRun` inherited stale `parentRun.modelName` and often forced an unnecessary respawn.

## Changes

- `apps/local-runner/internal/runner/runner.go`
  - Replaced singleton `grokProcess` with `grokProcesses map[string]*grokProcessHandle` keyed by `grokProcessKey(scope, model, effort, alwaysApprove)`.
- `apps/local-runner/internal/runner/grok_process.go`
  - Same-scope processes with different launch tuples now coexist; account/scope change still reclaims prior-scope handles; `closeAllGrokProcesses` clears all handles on YOLO posture change.
- `apps/local-runner/internal/runner/interactive_service.go`
  - `runTurn` persists per-turn model and reasoning effort onto the parent run before child spawn can read them (mirrors BUG-129 YOLO stickiness).
- Tests: `grok_registry_test.go`, `grok_process_test.go`, `grok_mcp_test.go`, `grok_adapter_test.go`, `interactive_service_test.go`.

## Verification

- `go test ./internal/runner -run 'TestGrokMcpSpawn|TestEnsureGrokProcess|TestGrokParentTurnModel|TestApplyGrokYolo'` — PASS.
- Live DOD-2 E2E: **confirmed by user 2026-07-11** — child agent spawned during Grok parent turn without `grok agent process torn down`; child visible in agent panel.

## Follow-ups

- Task-209 remaining: DOD-4/5/7/8/9.
- Task-221 adjacent: MCP tool gating under YOLO=off is separate from this process-lifecycle fix.

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: Task-209
change_type: bugfix
summary: Grok spawn_agent no longer tears down parent grok agent process — keyed multiprocess coexistence plus per-turn model/effort inheritance for child spawns
# --->8---