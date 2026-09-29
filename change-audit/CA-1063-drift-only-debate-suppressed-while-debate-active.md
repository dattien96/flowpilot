# CA-1063 — drift-only owner-debate re-mount suppressed while a debate is active

## Symptom (live run-15525, CA-1062 verification)

With the CA-1062 restore/reprompt seam in place, a vibe sprint that routed
into `vibe-owner-debate` never came back: every `debate_synthesis` turn —
an inline hub turn that deliberates and writes no files — tripped the
drift detector's `zero_delta_progress` signal (scores 80→100), and
`applyVibeDriftOnlyResolver` mounted a **fresh** owner debate on top of the
running one. The cycle `synthesis → drift flag → re-mount → synthesis`
never converged; the parked sprint chain (and the CA-1062 reprompt intent)
could never resume, and there is no cap on drift-driven debate mounts
(`vibeOwnerFailRetries` only counts owner-*failures* — owners completed
fine every round).

## Root cause

`applyVibeDriftOnlyResolver` fired on any clean-gate vibe turn at
drift ≥ threshold without checking whether the flagged turn *belongs to*
the remediation flow itself. Deliberation turns (debate synthesis, owner
delegates' coordination legs, post-restore resume legs) legitimately
produce zero file deltas — for them `zero_delta_progress` is a signal
misfire, not stall evidence.

## Fix

`applyVibeDriftOnlyResolver` now resolves the hub run first and suppresses
the escalation when the hub is already inside the owner-debate
remediation:

- `len(hubRs.vibeParkedNodes) > 0` — sprint topology parked for a debate,
  covering the whole debate window (stash → restore).
- `workingmode.BareFlowID(hubRs.chatFlowRef) == vibeOwnerDebateFlowID` —
  the debate flow is still the active chat flow (e.g. a debate-flow turn
  settling after restore cleared the parked state, before the verdict
  edge resolves).

The suppressed turn falls through to the normal completion path, so the
synthesis node's `done` edge fires → `onVibeCpNodeDone` →
`restoreVibeFlowAfterDebate` → CA-1062 reprompt/resume. Same precedent as
the existing drift-ladder suppression for in-debate turns
(`recordDriftTelemetry`, gate_hook.go).

Residual surface (documented, bounded): a verdict-compliant zero-delta
confirm turn on a *sprint* child can still mount one debate per reprompt
cycle — bounded by the existing reprompt/escalation caps, converging to an
operator-visible escalation rather than an unbounded loop.

## Tests

- `TestVibeDriftOnlyResolver_SuppressedInsideDebate` (new): parked sprint
  topology → drift 95 suppressed.
- `TestVibeDriftOnlyResolver_SuppressedOnDebateFlowTurn` (new): debate
  `chatFlowRef` still active → suppressed.
- `TestVibeDriftOnlyResolver_ChildSuppressedWhenHubInDebate` (new): a vibe
  child's clean-gate drift resolves suppression on the HUB's debate state.

## Verification

- `go test -count=1 -run 'TestVibeDriftOnlyResolver_|TestVibeGatePrecedence_|TestStashVibeFlowForDebate|TestVibeDebate' ./internal/runner/` — ok.
- Live evidence before fix (run-15525): `[drift] score=80 signals=[zero_delta_progress] action=pause_for_human` →
  `[vibe-gate] drift-only escalation -> owner debate` at 20:07:00 and again
  score=100 at 20:09:00, owner cohorts re-spawning each round with the
  parked sprint never restored.
