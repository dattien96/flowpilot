# BUG-627 — Hub turn writes bypass the step-scoped frozen-contract guard

- **ID:** BUG-627
- **Severity:** High (contract enforcement has a structural hole: the main
  session can silently do any step's work unbound)
- **Status:** open
- **Found:** live run-174243 (Task-033 sprint), 2026-10-03 ~23:19–23:22

## Symptom

A reprompt landed on the hub turn (`turn-178794`, "Feedback received on
your last submission… resubmit"). The hub declared *"I'm now the
materializing node under contract a49995cb"* and wrote, from the main
orchestrator session:

- `protected_key_guard_test.cpp` (new)
- `protected_key_guard_contract_test.cpp` (new)
- `run_guard_tests.sh` (new)
- **`VaultContainer.h` — edited, ProtectedKeyGuard class added**

`a49995cb` freezes `VaultContainer.h`/`.cpp` read-only for the tdd step —
a child leg labelled `tdd` would have been blocked at write time. The
hub carries no step label, so the contract binding never applied; the
edit was only caught **post-hoc** by the planner-purity fingerprint
(which then fed BUG-626's contradictory reprompt loop).

## Defect

Contract enforcement binds by step identity (`GetFrozenForStep`,
label-keyed) — it guards *delegate legs*, not the hub's own file
mutations. Two gaps fall out:

1. **Write-time**: hub turns can mutate contract-frozen paths with no
   label to bind against — enforcement is post-hoc only.
2. **Role collapse**: a reprompt addressed to a hub verdict can convert
   the hub into a worker ("materializing node") — doing tdd+coder work
   with the full tool surface, outside the scaffold/coder posture the
   sprint topology intends.

## Expected fix direction

- Extend contract checking to hub-originated file writes: when a frozen
  contract is active for the run and the hub turn touches a path on its
  read-only/frozen list, block (or require the owning step's delegation
  edge) rather than flagging after the fact.
- Alternatively/normatively: reprompt routing should never hand artifact
  work to the hub — the reprompt should redirect to the owning step's
  leg (paired with BUG-626's scope-aware remediation).
- Provider-parity: this binds writes by run/step metadata, so it is
  provider-agnostic — verify once, note the evidence.
