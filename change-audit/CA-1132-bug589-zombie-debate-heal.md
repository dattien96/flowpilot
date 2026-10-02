# CA-1132 — BUG-589: Zombie owner-debate claim swallows every gate divert

## What changed

`apps/local-runner/internal/runner/vibe_debate.go`:

- New `maybeResolveZombieVibeDebate(parentRunID)` — heals a debate overlay
  that is claimed but dead: `vibeParkedNodes` still owns the real flow while
  nothing can emit the `done` that unmounts it, so every subsequent gate
  divert returns `handled=true` from the suppress branch and the gated
  completion is orphaned.
- Healing fires only on unambiguous terminal shapes (the same shapes the
  restart path resolves in BUG-567):
  - `debate_synthesis` DONE but restore missed → replay
    `restoreVibeFlowAfterDebate` (idempotent).
  - both owners DONE with synthesis never started → re-drive the synthesis
    hub via `maybeResumeVibeDebateSynthesis`.
- Any live member (running owner, running/waiting synthesis, pending
  decision) leaves the debate alone — suppression stays suppression.
- Owner-fail / starved / lopsided shapes stay with `maybeSettleVibeOwnerDebate`.

`apps/local-runner/internal/runner/vibe_gate.go`:

- `startVibeOwnerDebate` suppress branch now calls
  `maybeResolveZombieVibeDebate(hub)` — a divert that lands on a dead claim
  heals it instead of silently dropping the gated run.

`apps/local-runner/internal/runner/wedge_sweep.go`:

- `sweepWedgedFlowWork` collects runs with non-empty `vibeParkedNodes` and
  runs the same heal, so a zombie debate resolves even when no new divert
  arrives (live run-139670 sat wedged 30+ min with zero inbound turns).

## Live evidence (run-139670)

- Owner cohort join mis-dispatched into the sprint's `synthesis` step at
  00:26:59 (`hub_reinvoke_start_failed`); `debate_synthesis` never ran.
- Coder `turn_completed` at 00:35:49 and again 01:07:55 → gate →
  "owner debate mount suppressed: debate already active" → orphaned; node
  pinned RUNNING 15+ min per occurrence.
- Two HTTP bypasses (`flow-control continue`, `gate_mode=warn` + direct
  turn) were required to unblock — exactly the class of intervention this
  heal removes.

## Invariant

A debate claim must be backed by a debate that can still conclude. If the
claim outlives the debate, the runner restores the parked flow or
re-drives synthesis — it never eats the gated outcome.

## Tests

`internal/runner/bug589_zombie_debate_heal_test.go` — red-first:

1. zombie debate (owners DONE, synthesis never ran, parkedNodes held) +
   gate divert → synthesis hub re-driven, gated run preserved.
2. zombie debate with synthesis DONE but no restore → restore replays.
3. live debate (synthesis running) → suppress branch leaves it untouched.
