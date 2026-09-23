# BUG-379: `devin/swe-2-max` rejected `Invalid params` → silent fallback to `swe-2-high` with no user-facing signal

## Metadata

- Document ID: `BUG-379`
- Title: `Requested devin model id not in live catalog → set_config_option Invalid params → turn runs on swe-2-high silently`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-70-Devin-Provider-Integration](../../07-Coding-Plan/done/CP-70-Devin-Provider-Integration.md), [CP-41-RAG-Harness-Flow-Mode](../../07-Coding-Plan/done/CP-41-RAG-Harness-Flow-Mode.md)
- Feature Keys: `ai-providers`

## AI Quick View

### Summary

- A run created with `model:"devin/swe-2-max"` issues `session/set_config_option{configId:"model", value:"swe-2-max"}`; devin returns `Invalid params` (`swe-2-max` is absent from the live `configOptions` catalog — `swe-2-high`, `swe-1-7-*`, `adaptive` exist; "max" exists only as a *thought_level*).
- The turn then proceeds on `swe-2-high` (`configOptions.currentValue`) with **no user-facing signal** that the requested model was not applied.
- Inconsistent reports across waves: CP-70 and CP-41 observed the rejection+fallback, while CP-58/a CP-41 preflight showed `swe-2-max` applied — the model-id mapping (requested id → catalog id vs thought_level) needs investigation.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.
- Investigation note: verify whether `swe-2-max` should map to `swe-2-high` + `thought_level:"max"`, and surface a warning event when the requested model is rejected.

## Bug report

- **Symptom**: Runs requested on `devin/swe-2-max` actually execute on `swe-2-high`; nothing in the SSE/admin surface tells the user the model was not applied.
- **Expected**: Either the model applies, or the runner emits a visible warning/turn note that the requested model id was rejected and a fallback is in use.
- **Actual**: `session/set_config_option` → `Invalid params`; `admin-events` config options report `currentValue:"swe-2-high"`; all turns complete normally on the fallback model.
- **Impact**: low — silent capability downgrade (max→high) can skew results and makes model-sensitive verification untrustworthy, but turns still execute.

## Reproduction

1. Runner `FLOWPILOT_DEVIN_AGENT=1` (cp70 :19270).
2. `POST /client/workflow-runs` `{providerKey:"devin", model:"devin/swe-2-max", …}` (cp70 run-1 et al.; cp41 all runs).
3. `runner.log` — `session/set_config_option{model:"swe-2-max"}` → `Invalid params` error frame; subsequent ACP `configOptions` show `currentValue:"swe-2-high"`.
4. Send a turn — it completes on swe-2-high; no rejection surfaced to the user.

## Root cause

- Model-id mapping gap: FlowPilot passes the catalog-level model id (`swe-2-max`) straight to `session/set_config_option`; devin's live `configOptions` catalog does not contain that id (per cp41: "max" exists only as a *thought_level*). The `Invalid params` result is swallowed — no propagation to the turn/run event stream. Exact mapping site to confirm (devin adapter model selection / set_config_option path).

## Evidence

- `~/fp-beds/lt-evidence/cp70/RESULT.md` (BUG-LIVE-CP70-3), `runner.log` (set_config_option error), `r3-admin-events.json` (`currentValue:"swe-2-high"`).
- `~/fp-beds/lt-evidence/cp41/RESULT.md` (header/scope note: `devin/swe-2-max` absent from live catalog — configOptions lists `swe-2-high`, `swe-1-7-*`; all CP-41 runs used `swe-2-high`; recorded as scope limitation but consistent with this bug).
- Prior waves: CP-58 and CP-41 agents observed the same; one preflight reportedly showed `swe-2-max` applied — inconsistent, needs a definitive mapping check.

## Severity

- low

## Completion Notes (implemented 2026-09-22, CA-916)

- Root cause: `session/set_config_option` Invalid-params rejection was log-only; the turn ran on the session's previous model with no user-facing signal while records kept claiming the requested id.
- Fix (shared fix with BUG-433): applied-model tracking — `appliedModel` per-session map seeded from `configOptions[].currentValue` on session/new|load and refreshed on accepted set_config_option; rejected or coerced values emit a `[model]`/`[mode]` message delta; `recordAppliedSessionModel` re-upserts the session record with the applied (not requested) model id.
- Files: `internal/runner/devin_adapter.go`.
- Tests: `bug379_devin_model_fallback_test.go`.
