# BUG-361: OpenCode quota/limit hang — scout stays RUNNING, no turn_failed

## Metadata

- Document ID: `BUG-361`
- Title: `OpenCode quota/limit hang — scout stays RUNNING, no turn_failed`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-07`
- Last Updated: `2026-09-07`
- Feature Keys: `ai-providers`
- Parent Documents: CP-57 (OpenCode provider)
- Child Documents: `none`
- Related Documents: [CA-758](../../../change-audit/CA-758-Run-207435-No-Repark-After-Freeze-Done.md) (unrelated live retest that surfaced this), Claude usage-limit mapping (`claude_event_mapper.go`, `claude_usage.go`)
- Replaces: `none`
- Tags: `opencode, quota, hang, turn-failed, adapter`

## AI Quick View

### Summary

- Live `run-208380` (2026-09-07, TUI Task Harness, `opencode/muse-spark-1.3-contributor-free`): scout `preflight_contract_plan` stays RUNNING / TUI "Thinking 2m+" while the operator already knows the model is over usage limit.
- Expected: same class as Claude — `EventTurnFailed` with a usage-limit message, child FAILED, loop parks with an actionable card. Actual: `session/prompt` never returns; child `run-208385` stays `running`; parent loop `running`; no `turn_failed`.
- Parent hub watchdog cannot fire: `hasActiveFlowChild` is true while the scout is still "running". Hang is silent until `/stop`.

### Current Ask

- Capture only. Do **not** implement in this filing. Plan the lean adapter fix + additive tests below; implement in a later turn.

### Key Decisions

- `D-1` Capture-only at filing (same pattern as BUG-360/BUG-357 at open).
- `D-2` Scope = OpenCode adapter + usage-limit classifier. Not CP-61, not CA-758 re-park, not TUI chrome.
- `D-3` Workflow: **lean** (single adapter path, known Claude/Grok precedent, no topology change).

### Constraints

- safe-fix-contract: additive tests only; no old-test edits; Claude/Codex/Grok classifiers must not regress (R2: this is Case 2 — shared `isProviderUsageLimitError` + per-adapter mapping).
- Must not treat a slow-but-healthy `session/prompt` as quota (false-positive fail).
- Must not weaken Claude's existing usage-limit mapping.

### Open Questions

- `Q-1` Exact ACP payload when muse-spark is over quota: RPC error on `session/prompt`, or a `session/update` with stopReason/error and no RPC return? Capture one live frame before coding the mapper.
- `Q-2` Bound: if `session/prompt` never returns at all, is a turn-level timeout required in addition to error mapping? Separate from mapping a returned error.

### Source Refs

- Live: `run-208380` / child `run-208385` (`GET /client/workflow-runs/run-208380/agent-graph`: loop `running`, scout `running/running`).
- `apps/local-runner/internal/runner/opencode_adapter.go` `SendTurn` (~257–376): waits on `dispatcher.call(ctx, "session/prompt", …)` with no quota branch.
- `apps/local-runner/internal/runner/opencode_event_mapper.go`: **no** quota/402/credits/`usage limit` tokens (contrast `claude_event_mapper.go` ~180–189, `claude_usage.go`).
- `apps/local-runner/internal/runner/interactive_service.go` `isProviderUsageLimitError` (~7767): Claude/Grok strings only; OpenCode hang never reaches it because `SendTurn` has not returned.
- `apps/local-runner/internal/runner/hub_stall.go` `checkAndBlockStalledHub`: `hasActiveFlowChild` re-arms and returns false — parent cannot `hub_stalled` while scout is still RUNNING.

## 1. Issue Summary

Operator starts Task Harness on a known-exhausted OpenCode Zen model (`muse-spark-1.3-contributor-free`). Scout step stays RUNNING and the TUI shows Thinking for minutes. No error card, no FAILED step, no quota copy. Claude on the same class of error fails the turn immediately with "usage limit reached…".

## 2. Parent Links

- impacted coding plan: CP-57 OpenCode provider
- impacted tech design: none (adapter mapping gap)
- impacted system spec: none

## 3. Environment and Reproduction

- environment: Windows, TUI `just chat-dev D:/working/gate-sandbox`, runner `:4317`, provider OpenCode Zen, model `opencode/muse-spark-1.3-contributor-free` (operator-confirmed over quota).
- reproduction steps:
  1. `/flow task` → task-harness, `/new`.
  2. Prompt: `Change Divide(a,b) in calc.go to panic on zero divisor`.
  3. Observe `preflight_contract_plan` RUNNING / Thinking past ~2 minutes with no stream and no fail.
- frequency: every time this model is over quota (operator statement); confirmed once live (`run-208380`).

## 4. Expected vs Actual

- expected: turn fails fast with a usage-limit `EventTurnFailed`; scout step FAILED; loop blocked with Retry/Stop and a GateReason naming quota/limit; operator can switch provider and `/new`.
- actual: `session/prompt` outstanding; scout `running`; loop `running`; no `turn_failed`; `/stop` is the only exit.

## 5. Impact

- users affected: anyone on OpenCode when the selected model/account is over quota or rate-limited.
- workflows affected: any flow whose first delegate (scout) uses that model; also later nodes if the same model is reused.
- severity: high for OpenCode live testing (silent hang, false "Thinking"); not a data-loss bug.

## 6. Root Cause

- hypothesis: OpenCode ACP either (a) returns an error/`stopReason` that the mapper ignores, or (b) never completes `session/prompt` on quota. Classifier `isProviderUsageLimitError` never runs because `SendTurn` has not returned. Parent watchdog is correctly suppressed by a live child.
- confirmed cause: not fully confirmed until Q-1 (one ACP error frame). Code-conformant: mapper has zero quota tokens; `SendTurn` has no timeout besides `ctx`; hub stall skips when `hasActiveFlowChild`.
- evidence: live graph `run-208380`; grep of `opencode_event_mapper.go` vs Claude mapper; `SendTurn` select loop; `checkAndBlockStalledHub` child-busy branch.

## 7. Fix Strategy

- `F-1` Map OpenCode quota/limit (RPC error and/or `session/update` stopReason) to `EventTurnFailed` with a stable message, then return from `SendTurn` so `finishTurn` settles FAILED. Prefer extending `isProviderUsageLimitError` additively with OpenCode signatures once Q-1 is known — do not copy Claude strings blindly.
- `F-2` (only if Q-1 shows `session/prompt` never returns): add a bounded wait distinct from healthy generation (must not collide with CA-708 activity budget 15s re-arm). Fail closed with the same usage-limit event if the bound fires **and** a quota signature was seen, else a generic "provider turn stalled" fail — do not invent quota copy without a signal.
- Out of scope: changing hub_stall to ignore live children; CP-61/CA-758; TUI Thinking chrome.

## 8. Validation

- `V-1` Fake ACP: quota RPC error on `session/prompt` → one `EventTurnFailed`, `SendTurn` returns, scout not left RUNNING. Matrix not required on the mapper unit if Case 2 is proven by reading Claude/Codex/Grok classifiers unchanged + one OpenCode table test; still run Claude, Codex (`codex_event_mapper_test.go` usage-limit cases), and Grok existing usage-limit tests to lock R2.
- `V-2` Fake ACP: healthy slow generation (usage_update then text, existing activity-budget test) still completes — no false quota fail.
- `V-2b` Whichever quota variants Q-1 confirms (RPC error and/or `session/update` stopReason/error): table-test each variant to the same `EventTurnFailed`. A variant that exists without a test ships the same gap again.
- `V-2c` If `F-2` timeout is implemented: bound fires + quota signature → usage-limit fail; bound fires + healthy stream → no fail (distinct from quota copy).
- `V-3` Manual: exhausted muse-spark → FAILED + card, not Thinking hang. Switch provider → CA-758 retest proceeds.

## 9. Regression Guard

- tests: new file `bug361_opencode_quota_hang_test.go` (additive). Re-run `TestOpencodeActivityKeepsBudgetPastInitialCap`, Claude usage-limit mapper tests, `TestIsProviderUsageLimitErrorBaseRegressionPlusGrok402`.
- alerts: none.
- audit checks: CA after implement; do not file CA on this capture-only doc.

## 10. Follow-Up Document Updates
- notes left unchanged on purpose: CA-758 residuals; CP-61 P-2/P-3.

## Completion Notes (implemented 2026-09-07, CA-759)

- Implemented `F-1` (returned-error mapping): `isProviderUsageLimitError` gains ACP-shaped tokens (`rate_limit`/`rate-limit`/`rate_limited`, `quota exceeded`/`quota_exceeded`, `insufficient credit*`, `payment required/*`); `opencodeStopReasonToEvent` maps quota stopReasons to `turn_failed` via new `opencodeIsQuotaStopReason` (unknown reasons still complete); `SendTurn` consumes a quota RPC error into one stable `OpenCode usage limit reached: …` event + nil (codex pattern); quota stopReason terminal carries the same copy. Files: `interactive_service.go`, `opencode_event_mapper.go`, `opencode_adapter.go`.
- `F-2` timeout NOT implemented: no evidence the prompt never returns vs returns-an-error (dispatcher delivers JSON-RPC errors to the waiter; runner was down before a re-query could confirm). A blind bound would risk killing healthy slow turns and colliding with the CA-708 activity budget. Residual: a truly outstanding `session/prompt` still hangs until `/stop`.
- Tests (5, `bug361_opencode_quota_hang_test.go`): classifier table (new + old true, healthy false incl. empty), stopReason table (quota fail, healthy/unknown complete, old failures intact), fake-ACP quota RPC error → 1 non-recoverable `turn_failed` + nil, non-quota RPC error → err passthrough unchanged, quota stopReason result → `turn_failed` never completed.
- R1: `TestOpencodeProcessEnvIsolatesWindows` fails identically on the clean tree (Windows env path expectation, zero overlap) — pre-existing, untouched. Everything else in the parity run green.
- R2: Case 2 — shared classifier extended additively; OpenCode-only mapping; Claude/Codex/Grok existing usage-limit suites run green.
