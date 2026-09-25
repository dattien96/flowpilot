# BUG-377: Devin session stays `status:running` after turn → queued gate reprompts never dispatch (intermittent)

## Metadata

- Document ID: `BUG-377`
- Title: `Devin provider session never reports completed → durable reprompt intents queue forever; remediation loop silently dies`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-35-Context-And-Regression-Engine-Rollout](../../07-Coding-Plan/done/CP-35-Context-And-Regression-Engine-Rollout.md), [CP-37-Prompt-Context-Continuity](../../07-Coding-Plan/done/CP-37-Prompt-Context-Continuity.md), [CP-70-Devin-Provider-Integration](../../07-Coding-Plan/done/CP-70-Devin-Provider-Integration.md)
- Feature Keys: `workflow-runtime, context-regression-engine, ai-providers`

## AI Quick View

### Summary

- After a devin turn completes, the provider session remains `status:"running"` (`/admin/workflow-runs/<run>/provider-sessions`), while grok's identical session reports `completed`. Queued durable gate-reprompt intents therefore never flush — dispatch is gated on the session being idle (`turn_in_progress` forever).
- cp35 run-1: `r-additive-tests` reprompt (turn-1360) logged `[gate] reprompt attempt=0` then nothing for 4+ min; an earlier `r-newtest` reprompt (turn-3) was still pending 32 min later and was only settled by a user turn superseding it (`settle_superseded_reprompt`).
- cp37 run-1: `r-fk` reprompt persisted as `pending_gate_reprompt_prompt` gen=1 sat 13+ min while a user probe turn (turn-1973) dispatched normally; intent still pending (gen=1) at final snapshot.
- **INTERMITTENT**: the identical path on cp37 run-1981 dispatched reprompt turn-2783 within the same second; CP-70 did NOT reproduce on this build (reprompts dispatched and completed: run-1 turn-47, run-807 turns 1270/1632). State-dependent wedge.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.
- Investigation note: correlate the wedge with whatever holds the devin ACP session in `running` (warm held session vs. missing terminal status mapping); check whether `turn_completed`→session-idle transition is provider-specific.

## Bug report

- **Symptom**: A `flow_gate_violation` with `action=reprompt` fires correctly, the reprompt prompt is persisted durably, but no reprompt turn is ever dispatched to devin — the remediation loop silently dies; user sees a block with no follow-up.
- **Expected**: Queued reprompt intent flushes within seconds (grok control cp35 run-745 dispatched reprompt turn-954 immediately).
- **Actual**: `[gate] reprompt attempt=0 stepID="chat-run-1"` logged; no `session/prompt`, `[turn]`, or `[resume-intent]` line ever follows; the run stays `running` and the intent is only cleared when a user turn supersedes it (`settle_superseded_reprompt` in `dispatch.ndjson`).
- **Impact**: HIGH (when it fires) — every gate reprompt on the affected session is lost: violations are detected and announced but remediation never reaches the agent; the durable intent can sit 13–32+ min until superseded.

## Reproduction

1. Runner `FLOWPILOT_DEVIN_AGENT=1`, provider `devin`/`devin/swe-2-max`, gate `enforce`.
2. Produce any reprompt-class violation on a devin run: cp35 run-1 turn-1360 (edit inside `TestAddBy2` → `r-additive-tests` reprompt) or cp37 run-1 turn-1512 (commit with bad feature key `utils` → `violations=2 rules=[r-fk r-newtest]`, `action=reprompt`).
3. Observe `runner.log` `[gate] reprompt attempt=0` (cp37 `runner.log:2390-2392` @ 23:36:46) and `GET /admin/workflow-runs/run-1/provider-sessions` → devin session `status:"running"` (vs grok `completed`).
4. Wait: no reprompt turn starts (cp37: 13+ min, intent gen=1 still pending at snapshot; cp35: 4+ min / 32 min superseded). `dispatch.ndjson` has no reprompt turn records for run-1.
5. Negative control: cp37 run-1981 r-task reprompt dispatched `turn-2783` same-second (`runner.log:3568-3571`); cp70 run-1 turn-47 / run-807 turns 1270/1632 dispatched normally.

## Root cause

- Provider-session status mapping for devin never transitions to `completed`/`idle` after the turn settles → durable reprompt intents queued by the gate remain blocked behind `turn_in_progress` indefinitely. Exact status-write site not yet isolated; wedge is state-dependent (did not reproduce on cp70 build or cp37 run-1981).
- Interaction points to inspect: provider session status write path (`provider_sessions` admin surface), gate reprompt durable-intent flush (`[gate] reprompt attempt=0` → `pending_gate_reprompt_prompt` gen=N), and `settle_superseded_reprompt` (the only observed exit).

## Evidence

- `~/fp-beds/lt-evidence/cp35/RESULT.md` (BUG-LIVE-003), `dispatch.ndjson` (no reprompt records for run-1), `run1-gate-violations.json`, `runner.log` L805, L2853-2854 (`[gate] reprompt attempt=0` then silence).
- `~/fp-beds/lt-evidence/cp37/RESULT.md` (BUG-LIVE-CP37-002), `gate-log-greps.txt`, `run-1-events.json` (violation evt 16:36:46Z, no subsequent reprompt `turn_started`), `l372-reprompt-queued.txt` (full persisted reprompt text incl. suggested keys `calc, calc-core, claude`).
- `~/fp-beds/lt-evidence/cp70/RESULT.md` (CP35-003 disposition: NOT reproduced this build — reprompts dispatched run-1 turn-47, run-807 turns 1270/1632; note `provider_sessions` row for run-1 still reads `status:"running"` after completion — "reflects the warm held acp session, not a stuck turn").

## Severity

- high (intermittent — demote to medium if triage shows a narrow trigger)

## Completion Notes (implemented 2026-09-22, CA-916b)

- Root cause (two stacked defects, cp37 run-1 live evidence): (1) the post-turn-gate blocked path only dispatched a queued reprompt when `pendingFlowGateSettle` was armed — which requires a `file_changed` event or flow-driven run; Devin emitted none (BUG-375), so the reprompt was dropped with only a single-shot tail idle-flush. (2) When `claimDurableIntentLocked` failed (same-gen lease held by a wedged/never-returned startTurn goroutine), no path re-armed delivery — the 30-min lease blocked every later flush with zero watchdog.
- Fix: (a) unarmed-settle blocked branch now persists the reprompt checkpoint and dispatches via `scheduleRootGateRepromptOrPark`/`startTurnClearingIntent` identically to the armed branch (persist failure still blocks dispatch + backs off the gen); (b) `durableIntentRearmProbe` (15s) — a failed claim arms a bounded one-shot `notifyTurnIdle` that re-samples state and self-terminates once the intent clears or delivers.
- Files: `internal/runner/interactive_service.go`, `internal/runner/interactive_resume.go`.
- Tests: `bug377_reprompt_strand_test.go` — wedged-claim reclaim, unarmed-settle dispatch, expired-lease reclaim. Provider-agnostic durable-intent plumbing (no providerKey branches).
