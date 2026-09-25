# BUG-433: Devin invalid model id silently ignored; run metadata claims the requested model

## Metadata

- Document ID: `BUG-433`
- Title: `devin set_config_option Invalid params logged-only; turn runs on session default while records claim requested model`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-46-Test-Steps](../../07-Coding-Plan/done/), [BUG-379](BUG-379-Devin-Swe2Max-Invalid-Params-Silent-Fallback.md)
- Feature Keys: `provider-devin`, `agent-flow-engine`

## AI Quick View

### Summary

- Run created with `model:"devin/swe-2-max"` (absent from the live catalog — devin 3000.11.1 only offers `swe-2-high`). Every turn issues `session/set_config_option{value:"swe-2-max"}` → `-32602 Invalid params`. Error is **log-only** (`devin_adapter.go:683`); the turn proceeds on the session's existing model (`currentValue:"swe-2-high"`, `modelLabel:"SWE-2 High"`), while runner records keep logging `resolved_model="devin/swe-2-max"`. Silent wrong-model execution, repeated every turn.
- Extends BUG-379 (silent fallback) with the stronger defect: the runner's own records lie about which model actually ran.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom**: requesting `devin/swe-2-max` yields turns that actually run on `swe-2-high`; no error surfaces to the client event stream.
- **Expected**: invalid model id → `turn_failed` or at minimum a surfaced warning + recorded model corrected to the applied value.
- **Actual**: `-32602` swallowed at `devin_adapter.go:683`; `resolved_model` keeps the unapplied id for every turn of the run.
- **Impact**: users picking a stale/typo'd model silently run a different model; audit/metadata untrustworthy; same failure repeats each turn.

## Reproduction

1. Runner with `FLOWPILOT_DEVIN_AGENT=1`, create chat run with `provider=devin`, `model=devin/swe-2-max`.
2. Observe `session/set_config_option` → `Invalid value 'swe-2-max' for config option 'model'` in runner log; turn completes on `swe-2-high`.
3. Check `configOptions`/`_cognition.ai/agent_stopped` → `modelLabel:"SWE-2 High"`; runner log still says `resolved_model="devin/swe-2-max"`.

## Root cause

- `devin_adapter.go:683` — `set_config_option` error path logs but does not propagate or correct the recorded model.

## Evidence

- `~/fp-beds/lt-evidence/cp46/BUG-LIVE-CP46-R1-devin-invalid-model-silent-fallback.md`
- `~/fp-beds/lt-evidence/cp46/retest-runner.log` L49-51/L128-130/L188-190/L278-280; `r-run38-events.sse`, `r-providers.json`.

## Severity

medium

## Completion Notes (implemented 2026-09-22, CA-916b)

- Same root cause and fix as BUG-379 (dup report): silent fallback eliminated — rejection/coercion now surfaces a user-facing message delta and the provider-session record carries the applied model (`configOptions[].currentValue`), never the rejected id.
- Files: `internal/runner/devin_adapter.go`. Tests: `bug379_devin_model_fallback_test.go`.
