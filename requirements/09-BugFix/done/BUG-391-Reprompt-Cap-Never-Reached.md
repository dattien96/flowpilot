# BUG-391: repromptAttempts resets each new turn — reprompt cap never reached, infinite gate loops

## Metadata

- Document ID: `BUG-391`
- Title: `rs.repromptAttempts resets at every new turn → maxFlowGateReprompts never accumulates → unbounded reprompt loops`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-50-Context-Source-Completion](../../07-Coding-Plan/done/CP-50-Context-Source-Completion.md), [CP-66-Test-Steps](../../07-Coding-Plan/done/CP-66-Test-Steps.md), [CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor](../../07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md)
- Feature Keys: `agent-flow-engine`, `reproduce-first-gate`

## AI Quick View

### Summary

- `rs.repromptAttempts = 0` runs at every new turn start (`interactive_service.go:9023`), but each gate reprompt launches a **new** turn — so `maxFlowGateReprompts` never accumulates and gate reprompts loop forever; every logged reprompt shows `attempt=0`.
- Three independent live confirmations: CP-50 run-169 (6 reprompt generations), CP-42 run-442 (≥5 reprompts + 4 `ask_user` escalations), CP-66-3 run-4014 (≥4 reprompts on a **false premise** — "the suite passed, so the stubs contain real implementation" logged one line after the gate's own `[gate] suite end … err=exit status 1`).
- Each reprompt is a full provider turn — CP-66-3 burned ~50 min / 5 agent turns before an operator demoted `gate_mode` enforce→warn to unblock.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** Gate reprompt loops never terminate on their own — `attempt=0` on every reprompt record regardless of how many reprompts have already fired.
- **Expected:** Reprompt attempts accumulate across the gate loop so `maxFlowGateReprompts` caps the cycle and escalates/parks instead of burning unbounded provider turns.
- **Actual:** The counter lives on run state that is reinitialized per turn; each reprompt spawns a fresh turn which resets it to 0 → the cap is dead code.
- **Impact:** Any gate violation that reprompts (reproduce, scaffold-RED, tests) can loop forever — observed 6, ≥5, and ≥4 generations. Compounds BUG-389 (reprompt pressure is what induces fabricated RED) and BUG-390 (the wrong "suite passed" violation loops forever). Second symptom (CP-66-3): the scaffold gate reprompted on a false premise — its reprompt text asserted "suite passed" immediately after its own log recorded `err=exit status 1` — and could never exhaust/escalate.

## Reproduction

1. Trigger any armed reprompting gate rule in a live flow (e.g. `r-reproduce` via bug-harness, or scaffold RED gate via task-harness `test_signatures`).
2. Let the gate reprompt more than `maxFlowGateReprompts` times.
3. Observe: every reprompt logs `attempt=0`; the loop only stops via external intervention (interrupt, gate_mode demotion, or manual verdict injection).
- runIds: `run-169` (CP-50, gens 1–6 — `run1/dispatch.ndjson`), `run-442` (CP-42, ≥5 reprompts + q-928/q-1707/q-2198/q-4147), `run-4014` (CP-66-3, runner.log:6471/:7277/:8390/:10323, all `attempt=0`).

## Root cause

- `apps/local-runner/internal/runner/interactive_service.go:9023` — `rs.repromptAttempts = 0` executes at new-turn start; gate reprompts are delivered as new turns, so the attempt counter is re-zeroed before the cap check can ever see `>0`.
- No separate persistent reprompt ledger per gate/step exists — the cap (`maxFlowGateReprompts`) reads a counter that by construction never increments across the loop it is meant to bound.

## Evidence

- `~/fp-beds/lt-evidence/cp50/RESULT.md` (BUG-LIVE-01 step 5 — "Unbounded loop"; `run1/dispatch.ndjson` gens 1..6 all `attempt=0`; `run1/gate-metrics.ndjson`)
- `~/fp-beds/lt-evidence/cp66/RESULT.md` (BUG-LIVE-66-3 — ≥4 reprompts, `attempt=0` each, false-premise reprompt text; unblocked only by `.flowpilot/settings/gate-config.json` enforce→warn; `gate-config-before.json`)
- `~/fp-beds/lt-evidence/cp42/RESULT.md` (BUG-LIVE-1 — ≥5 reprompts, 4 ask_user cycles, manual interrupt)
- `~/fp-beds/lt-evidence/cp66/runner.log` — reprompts at :6471/:7277/:8390/:10323 each preceded by `[gate] suite end … err=exit status 1`

## Severity

- `high` — every reprompting gate is an unbounded provider-turn burner; combined with false-positive violations it deadlocks flows (CP-42/CP-50) or forces operator gate demotion (CP-66).

## Completion Notes (implemented 2026-09-23, CA-919)

- repromptAttempts survives reprompt-delivery turns (scenarioGateReprompt) and resets on gate pass — the cap now bounds the loop. Test: bug391_reprompt_cap_test.go.
