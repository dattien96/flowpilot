# BUG-393: Post-wedge chat turn writes production code with ZERO gate evaluation

## Metadata

- Document ID: `BUG-393`
- Title: `Plain chat turn on a wedged vibe sprint implemented stringutil.Reverse with no [gate] evaluation`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-49-Test-Steps](../../07-Coding-Plan/done/CP-49-Test-Steps.md)
- Feature Keys: `vibe-mode`, `agent-flow-engine`

## AI Quick View

### Summary

- After vibe run-2290's sprint wedged/settled (`done` at `vibeTaskIndex 1/3`, then `blocked: hub_stalled`), a plain `POST /turns {stepId:"tdd", prompt:"continue"}` + answering `q-8185` produced turn-8012 — a **plain chat turn** that implemented `stringutil/reverse.go` (rune-swap body).
- `runner.log` shows `[settle] finalized run=run-2290 turn=turn-8012` with **no `[gate]`/`[vibe-gate]` evaluation lines** — the TDD coder step was effectively executed outside the sprint flow, outside the frozen contract, with no audit/handoff provenance.
- The sprint's hard-ceiling contract (frozen contract, scope-drift, signature lock, audit trail) silently does not apply to post-wedge plain turns on a flow's `stepId`.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** A turn addressed to a sprint step id (`stepId:"tdd"`) on a wedged/completed vibe run runs as ungated plain chat and writes production code.
- **Expected:** Either the turn is refused (run/loop terminal or parked), or it re-enters the flow under the frozen contract and gate rules — never an ungated write.
- **Actual:** turn-8012 wrote `stringutil.Reverse` with zero gate lines; no frozen-contract scope check, no TDD RED requirement, no audit provenance. (`L49-2-run2290-reverse-diff.txt` is empty only because the written content coincidentally equals the baseline implementation.)
- **Impact:** The whole enforcement stack is bypassable by continuing a dead flow's step id — code lands with no contract, no gate, no CA/handoff trail. Same silent-degradation family as CP-49 BUG-LIVE-1 (flowRef-resolution fallback to ungated chat).

## Reproduction

1. Wedge a vibe-sprint run (run-2290: tdd owner-debate → loop `done` at task 1/3 → `blocked: hub_stalled`).
2. `POST /client/workflow-runs/run-2290/turns {"stepId":"tdd","prompt":"continue"}`; answer the surfaced question `q-8185` ("Implement stringutil.Reverse…").
3. Observe turn-8012 run as plain chat, write `stringutil/reverse.go`, settle with no `[gate]` lines.
- runIds: `run-2290` (turn-8012); parent context `run-9`, `run-1588` (same wedge family).

## Root cause

- Turn admission on a flow run does not check that the referenced step is live/gated — after the loop settles, `POST /turns` with a stale `stepId` degrades to an ordinary chat turn and no gate hook evaluates it (no `RunOracle`/rule pass in the log window 02:59:03→03:09:18). Underlying wedge (`hub_stalled`, lost verdict) is CP-49 BUG-LIVE-2/3; this bug is specifically that the post-wedge turn is ungated rather than rejected.

## Evidence

- `~/fp-beds/lt-evidence/cp49/RESULT.md` (BUG-LIVE-4; L-49-2 BLOCKED notes)
- `~/fp-beds/lt-evidence/cp49/runner.log` — turn start 02:59:03 → settle 03:09:18, zero `[gate]`/`[vibe-gate]` lines
- `~/fp-beds/lt-evidence/cp49/L49-2-run2290-turns.ndjson` — turn-8012 transcript
- `~/fp-beds/lt-evidence/cp49/L49-2-run2290-reverse-diff.txt` — empty diff (coincidental match to baseline)
- `~/fp-beds/lt-evidence/cp49/L49-2-run2290-agentgraph-final.json` — `loopState.status=blocked`, `blockReason=hub_stalled`

## Severity

- `high` — ungated production writes on flow runs; provenance and contract enforcement silently absent.

## Completion Notes (implemented 2026-09-23, CA-920b)

- `turnStartedAfterLoopDone` no longer suppresses post-turn gate evaluation; post-seal follow-up writes are now gated like any other turn. Admission semantics unchanged (blocked refused; done/stopped admit per BUG-302/308) and `pendingFlowGateSettle` stays unarmed per the BUG-305 pin.
- Unit+E2E: `TestBug393_BlockedLoopStillRefused`, `TestBug393_PostSealTurnDoesNotArmSettle`, `TestBug393_GateEpochValidForPostSealTurn`, `TestBug393_PostSealTurnStillGateEvaluated` (e2e violation observed) — all green; `TestV10FlowEngineRootAfterLoopDoneCompletesImmediately` regression pin green.
