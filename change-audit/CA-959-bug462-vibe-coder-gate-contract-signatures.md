# CA-959 — BUG-462: vibe coder gate accepts contract-pinned LockedSignatures as TDD evidence

- type: bugfix
- bug: BUG-462 (found live run-37268 — false `tdd artifact missing` park
  70ms after the scaffold pinned the coder contract's signatures)
- feature: agent-flow-engine

## Change

`internal/runner/vibe_sprint.go`:

- New `vibeTddEvidencePresent(parentRunID, coderStepID, cwd)`: filesystem
  artifact (`hasVibeTddOutput`) OR the active frozen contract for the coder
  step carrying non-empty `LockedSignatures`. Contract signatures are
  strictly stronger evidence — they only exist after the scaffold gate
  pinned them for this run, so a stale full-body test file cannot fake them.
- `resumeVibeAfterTddGate` and `maybeResumeVibeCoderAfterTdd` now call it.

`internal/runner/flow_executor.go`:

- The agent.code auto-advance path opens the frozen-contract store BEFORE
  the tdd-evidence gate, so the same `GetFrozenForStep` result feeds both
  `hasTddEvidence` (signatures count) and the no-contract park. Park
  reasons and ordering semantics preserved.

## Why

Fail-closed stays fail-closed: a pre-tdd v1 freeze has no LockedSignatures
and still parks; a full-body untracked test file with no contract still
parks (CA-769). Only the runner's own scaffold attestation newly counts —
it proves the sanctioned TDD step ran even when its output took the shape
of adopted full-body tests rather than `tdd-signatures.md`.

## Tests (additive only)

- `TestBUG462_AdvanceTddToCoderUsesContractSignatures` — forward advance.
- `TestBUG462_ResumeCoderUsesContractSignatures` — restart/resume path.
- `TestBUG462_FrozenContractWithoutSignaturesStillParks` — guard rail.
- `TestBUG462_ReadOnlyLockWithoutSignaturesSatisfiesGate` — extended in
  CA-960: `ReadOnlyPaths` on the coder contract (post-freeze lock with no
  signatures, the exact sprint-2 live shape after BUG-463) also counts.

## Provider parity

Provider-agnostic: pure gate/evidence logic before any provider call.
