# BUG-462: Vibe coder spawn gate ignores frozen-contract LockedSignatures — false park after real TDD

- status: done
- found: live run-37268 (vibe snake run, task-harness sprint for Task-3)
- fixed_by: CA-959
- tests: internal/runner/bug462_tdd_contract_signature_gate_test.go (3 tests)

## Symptom (live)

Vibe CP-ingest run completed cp_lock (user confirmed), task_slicer ran, and
the Task-3 sprint drove the full harness chain:
preflight_contract_plan → preflight_contract_freeze → context → tdd, all
DONE. The scaffold step then locked `snake/model_test.go` read-only and
pinned `signature_hash` + 10 `locked_signatures` onto the coder contract
(frozen_contracts.ndjson v2 record). But the tdd→coder advance parked:

```
vibe_tdd_missing  coder refused; tdd artifact missing
flow_parked_awaiting_user  flow frozen for human decision form
```

Loop blocked with `gateReason: "tdd artifact missing before coder (no
bypass)"` — a false park: TDD had provably run and the runner itself held
durable evidence.

## Root cause

`hasVibeTddOutput(cwd)` is a filesystem-only heuristic: it accepts
`tdd-signatures.md` or git-new `*_test.go` files that are signature-only
empty frames. Full-body test files are deliberately excluded (CA-769 —
an untracked file with `t.Fatal` assertions might be stale work the TDD
step never authored).

In this run the sanctioned tdd/scaffold step adopted a full-body test file
(its signatures were extracted and pinned onto the coder contract by the
scaffold gate). No `tdd-signatures.md` was written. The filesystem check
therefore returned false even though the runner's own contract store
carried stronger evidence: `locked_signatures` only lands on the coder
record AFTER the scaffold gate passed for this run's contract — a stale
file can never produce it.

The gate consulted the contract store one statement later
(`GetFrozenForStep`) but only for existence, not for `LockedSignatures` —
so the durable attestation was already open and unused when the park
decision fired.

## Fix

`vibe_sprint.go`: new `vibeTddEvidencePresent(parentRunID, coderStepID,
cwd)` — FS artifact OR active frozen contract for the coder step carrying
non-empty `LockedSignatures`. Applied at all three gate call sites:

- `flow_executor.go` auto-advance (tdd→coder): store opened before the
  evidence check so the same `GetFrozenForStep` result feeds both the
  evidence flag and the subsequent no-contract park.
- `resumeVibeAfterTddGate` (R-TK-K resume): same evidence check before
  deciding coder-resume vs tdd-restart.
- `maybeResumeVibeCoderAfterTdd`: same check before the missing-artifact
  park.

## Regression coverage

- `TestBUG462_AdvanceTddToCoderUsesContractSignatures` — contract-pinned
  signatures + full-body test file → tdd→coder advance spawns coder.
- `TestBUG462_ResumeCoderUsesContractSignatures` — same evidence through
  the restart/resume path.
- `TestBUG462_FrozenContractWithoutSignaturesStillParks` — guard rail: a
  pre-tdd freeze (v1, no LockedSignatures) still fails closed; the fix
  does not weaken the gate.
- `TestCA798_FullBodyTestsDoNotCountAsTddOutput` stays green — a full-body
  file with NO contract attestation still blocks.

## Live evidence

run-37268 diag: `vibe_tdd_missing` fired 16:04:21 immediately after
`scaffold: locked [snake/model_test.go] read-only + pinned signature hash
ab6692057932 for coder step "coder" (contract v2)` — the pin and the false
park were 70ms apart.
