# BUG-630 — Abandoned-leg `tdd-signatures.md` blocks poison later runs: freeze approves on evidence that does not exist

- **ID:** BUG-630
- **Severity:** High — this was the root trigger of the entire
  run-174243 cascade (premature freeze approval → gate challenge →
  owner debate → BUG-626 unwinnable loop → escalate → run stopped)
- **Status:** RESOLVED — CA-1159. Task-section scoping + runner
  attestation landed; regression tests in
  `internal/runner/bug630_stale_tdd_signatures_test.go` (RED→GREEN).
- **Found:** run-150388 sprint-3 leg abandoned mid-tdd (2026-10-02),
  surfaced in run-174243 2026-10-03 ~23:05–23:08

## Symptom

`requirements/.flowpilot/vibe/tdd-signatures.md` accumulates per-task
signature blocks. A Task-033 block written by a leg of **run-150388**
(cancelled before the task completed) survived in the file. On
run-174243:

1. `preflight_contract_plan`/`freeze` found the stale block for
   Task-033 and treated it as prior-art evidence — the freeze's
   `approved` verdict cited ProtectedKeyGuard signatures/paths that
   **did not exist in the working tree** (`VaultContainer.h` had no
   guard class).
2. The contract froze anyway (`a49995cb`) — with `VaultContainer.h` on
   the read-only list, matching the *stale* scaffold model rather than
   the actual tree state.
3. The tdd leg's gate/evidence check then challenged the verdict —
   mounted the owner debate at 23:08 — and everything downstream
   (BUG-624 dup, BUG-626 loop, BUG-627 hub write) was fallout.

## Defect

The signature ledger is a **persistent, run-unscoped** append log —
nothing ties a block to the run/contract that produced it or
invalidates it when its leg is abandoned/cancelled. Later runs read
orphaned blocks as authoritative prior art ("head-start"), so a
cancelled scaffold's *claims* become the next run's *evidence* — the
schema-first rule ("deterministic read → validate → fail-closed") is
violated by trusting prose artifacts over the tree.

## Expected fix direction

- Scope signature blocks to their producing run/contract: stamp each
  block with `run_id`/`contract_id`, and have freeze treat blocks whose
  contract was abandoned/superseded as history, not evidence.
- Freeze/planner evidence must resolve against the **working tree**
  (file exists + symbol present), not just the ledger — a cited
  signature whose file lacks the symbol is a validation failure, not an
  approval input.
- Leg abandonment should mark its ledger contributions `abandoned`
  (same posture as contract `abandoned` status already used by
  `9c85fd6f`/`94ea63b6`).

## Fix implemented (CA-1159)

- `vibeTddSignatureSections` splits the ledger on `# TDD sign*` headers;
  `vibeTddSignaturesTaskSection` resolves a `(Task-NNN)`-labelled block.
- `hasVibeTddSignaturesForTask` / `hasVibeTddOutputForTask` scope the
  artifact check to the current task's own section.
- `vibeScaffoldRedWaivedForTask` reads the waiver keys inside the current
  task's section only — another task's zero-red declaration no longer
  leaks across the file (whole-file last-wins stays for unlabelled files).
- **Provenance gate**: `vibeTddFileEvidencePresent` requires the run's
  durable `vibeTddSigAttestedTask` to match — set in the scaffold-turn
  gate block only when the leg actually WROTE `tdd-signatures.md` that
  turn (a stale block's mere presence cannot attest). Persisted via
  `ProviderSessionState.VibeTddSigAttestedTask` (ndjson + restore wired).
- Consumers rewired: `vibeTddEvidencePresent` (resume paths) and the
  flow-executor coder spawn gate; contract-lock evidence
  (LockedSignatures/ReadOnlyPaths) unchanged — still strictly stronger.
- Prompt hardening: `scaffold-contract-tdd.md` tells the leg stale
  sections are history and to declare its own `(Task-NNN)` section.

## Hygiene follow-up

Manually prune/annotate the stale Task-033 tail block in PrivateVault's
`tdd-signatures.md` (the committed task already superseded it).
