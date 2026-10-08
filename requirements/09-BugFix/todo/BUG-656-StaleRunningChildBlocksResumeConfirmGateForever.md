# BUG-656 — `maybeParkVibeResumeConfirm` bails on ANY child stamped `running` without checking provider liveness: a leg whose turn died at provider-limit keeps the parent `cancelled` forever — resume is a permanent no-op

- **ID:** BUG-656
- **Severity:** Critical — the run cannot be resumed by any operator
  route (`/resume`, `continue`, feedback) while a zombie `running`
  stamp exists; only settling the dead leg record unblocks it.
- **Status:** FIXED — CA-1236 (2026-10-08): resume-confirm gate checks live work (turnInFlight/agentActivity) instead of a stale running child status

## Evidence chain (all live)

1. Provider-limit killed TDD leg `run-584652` at 01:23Z; its last turn
   had completed at 01:21Z but the leg stayed `running` (BUG-647 settle
   gap). Parent resumed → `cancelled` — TWICE, both silent.
2. `maybeParkVibeResumeConfirm` (~interactive_service.go:851) treats any
  `running` child as live work → skips arming the resume-confirm gate →
   `normalizeResumedStatus` projects the incomplete parent to
   `cancelled` — a terminal-looking dead end with no card.
3. `interrupt` on the leg returned `cancelling` but never landed (no
   in-flight turn to cancel) — the zombie stamp survived even that.
   Only `flow-control {status:"done"}` on the leg settled it.

## Root cause

Resume-confirm's liveness test is the leg's status string, not actual
turn/provider liveness — a stamp that can never change on its own holds
the gate forever.

## Fix direction

- `F-1` Treat a `running` child with `turnInFlight=false` + no recent
  provider activity + completed last turn as dead — exclude from the
  "live child" guard (route it to BUG-647's settle sweep instead).
- `F-2` When resume finds a non-terminal parent with only dead children,
  arm the confirm gate normally; never land in unarmed `cancelled`.

## Regression coverage

- `TestBug656_DeadRunningChildDoesNotBlockResume` — leg stamped running
  with dead turn → resume-confirm gate arms.
- `TestBug656_LiveChildStillBlocks` — genuinely running child still
  defers the gate (no premature confirm).
