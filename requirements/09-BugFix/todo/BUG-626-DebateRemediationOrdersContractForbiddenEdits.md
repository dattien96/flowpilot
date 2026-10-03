# BUG-626 — Owner-debate remediation orders edits the frozen contract forbids → unwinnable reprompt loop → escalate

- **ID:** BUG-626
- **Severity:** High (parks the sprint on a human card every occurrence; burned ~30 min + 8 owner legs on run-174243)
- **Status:** open
- **Found:** live run-174243 (Task-033 sprint, PrivateVault), 2026-10-03 ~23:08–23:45

## Symptom

Recurring loop: `gated leg → owner debate → remediation reprompt → gate
re-flags the same condition → reprompt budget exhausted (BUG-570) →
escalate → user continue → same bind → debate round 2/3 → deadlock →
tournament rescue → tournament cancelled by park → hub_stalled card`.

Observed twice on run-174243 (escalate at 23:37:13, hub_stalled at
23:45:27) plus one earlier deadlocked cycle on run-150388.

## Defect

The debate/validation machinery can demand remediation that the frozen
contract structurally forbids — the two authorities disagree inside one
loop:

1. tdd contract `a49995cb` pinned `VaultContainer.h` **read-only**
   ("scaffold may only create NEW declared files as whitelist stubs").
2. The freeze's premature `approved` verdict was challenged; the
   deterministic reprompt (turn-178794) ordered: *"add the inline
   ProtectedKeyGuard class to VaultContainer.h exactly per the locked
   signature block in tdd-signatures.md"* — i.e., it ordered the very
   edit the contract freezes.
3. The edit landed → the planner-purity fingerprint flagged
   "production file(s) that existed at contract freeze modified —
   restore byte-for-byte" → reprompt → the remediation asked again for
   the file's contents → contradiction → exhaust → escalate.

Nothing arbitrates "the spec says the artifact lives in this file" vs
"this stage's contract freezes that file" — the debate's remediation
planner and the changecontract fingerprint run on disjoint scopes, so a
spec-correct placement becomes an unwinnable gate loop. The correct
answer existed in the same prompt (`9d507144`, the coder contract, has
no read-only list): the header edit belongs to the **coder** stage, not
to whatever agent happens to hold the reprompt turn.

## Expected fix direction

- Remediation synthesis must be **scope-aware**: before ordering an
  artifact change, resolve which step's contract owns the path and route
  the remediation to that step (here: defer the VaultContainer.h edit to
  the coder leg) instead of demanding it of the reprompt target.
- A reprompt that demands a contract-frozen path should be rejected at
  composition time — same fail-closed posture as the fingerprint, one
  hop earlier, with the conflict surfaced as a structured issue
  ("remediation conflicts contract <id>: path <p> frozen") rather than
  discovered by the gate after the write.
- Related hardening already landed: BUG-624 mount-window guard,
  BUG-621 sprint-scoped replay.
