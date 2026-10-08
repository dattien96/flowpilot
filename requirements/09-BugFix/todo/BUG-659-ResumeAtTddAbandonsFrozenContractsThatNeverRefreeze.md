# BUG-659 — Resume-at-`tdd` abandons the sprint's frozen contracts (`status:"abandoned"`, reason `vibe-sprint resume at tdd`) but the `preflight_contract_freeze` node has `lifecycle: once` and is already DONE — no contract is ever re-frozen, so every downstream writer hard-blocks on "no frozen contract found"

- **ID:** BUG-659
- **Severity:** Critical — turns a routine quota resume into a permanent
  gate wall: coder turn hits `flow_gate_violation` "no frozen contract
  found for step" with no operator-facing remedy documented.
- **Status:** OPEN (captured live, run-523131; abandonment event at
  01:04:09Z on contracts `50c2b302`/`9a5ad57f`, violation evt-594573)

## Evidence chain (all live)

1. Sprint-4 resume offered `resumeFrom: tdd`; on accept, both Task-114
   frozen contracts received `status:"abandoned"` events — presumably
   intent: freeze node re-runs and mints fresh ones.
2. `preflight_contract_freeze` is `lifecycle: once` and already DONE —
   the sprint resumed at `tdd` SKIPPED it → no new contract froze.
3. The redriven coder leg's change-contract lookup
   (`GetFrozenForStep(parentID, coderStepID)`) returned empty →
   hard-block; the run could not produce code until the operator
   appended `status:"frozen"` reactivation rows by hand into
   `frozen_contract_events.ndjson`.

## Root cause

Resume resets the STEP cursor but does not reset the once-lifecycle
freeze node, while contract lifecycle marks the old contracts abandoned
— the "re-freeze on resume-at-tdd" half of the design was never wired.

## Fix direction

- `F-1` If resume picks a step earlier than the freeze node, the freeze
  node's once-lifecycle must be re-armed (or the abandon must not fire).
- `F-2` Fail closed but tell the truth: gate violation should say
  "frozen contract abandoned by resume — re-freeze required" and offer
  a re-freeze action instead of a bare "no frozen contract found".

## Regression coverage

- `TestBug659_ResumeAtTddRefreezesContract` — resume before freeze node
  → new contract frozen before writer dispatch.
- `TestBug659_AbandonWithoutRefreezeBlocked` — no re-freeze path →
  explicit block event, not silent gate wall.
