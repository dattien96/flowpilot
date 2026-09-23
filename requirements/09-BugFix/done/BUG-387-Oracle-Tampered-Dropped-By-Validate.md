# BUG-387: oracle.Tampered dropped by runValidateWithOracleIfPossible — tampered locked test passes validate

## Metadata

- Document ID: `BUG-387`
- Title: `oracle.Tampered signal dropped by validate — tampered read-only test survives to run completion`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-67-Test-Steps](../../07-Coding-Plan/todo/CP-67-Test-Steps.md)
- Feature Keys: `context-regression-engine`, `contract-first-tdd`

## AI Quick View

### Summary

- Live run-19151 (CP-67 L-67-B): a marker was injected into the read-only `pageutil/page_test.go` after the coder turn; the tampered file survived to run completion and `validate` passed.
- `runValidateWithOracleIfPossible` consumes `flowgate.RunOracleContext` but maps only `SuitePassed` / `HasRegression` / `Failed` / `EnvError` into `ValidationResult` — `oracle.Tampered` is never propagated into the validation result or a flow violation.
- The read-only lock's tamper-detection signal therefore has no effect on the validate gate.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** A post-coder modification of a reproduce-locked test file does not fail or flag validation; the tampered file ships to run completion.
- **Expected:** `oracle.Tampered` non-empty surfaces as a validation failure or flow violation at the `validate` node (fail-closed on evidence tampering).
- **Actual:** The oracle collects `Tampered` (`oracle.go:19`, populated at `oracle.go:140`) but the runner-side mapping drops it; `ValidationResult` carries no tamper signal and no rule fires.
- **Impact:** Locked evidence files (reproduce tests, scaffold RED suites) can be modified after lock without the validate step noticing — silently undermines the reproduce-first and signature-lock guarantees. Compounds BUG-388 (lock already unenforced for absolute-path providers).

## Reproduction

1. Run a Contract-First Scaffold TDD flow that locks a test file read-only (CP-67 pageutil bed, run-19151).
2. After the coder turn, modify the locked test file (inject a marker) — see `l67b-run19151-page_test.go.tampered`.
3. Let the flow reach `validate`: observe `validate` passes and the run completes; no tamper violation/event.
- runIds: `run-19151`.

## Root cause

- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:508-589` — `runValidateWithOracleIfPossible` builds `ValidationResult` from `oracle.SuitePassed`/`HasRegression`/`Failed`/`EnvError`/`Message` only; `oracle.Tampered` is never read, so the tamper set is discarded.
- `apps/local-runner/internal/flowgate/oracle.go:19` — `Tampered []string` field exists and is populated (`oracle.go:140`); no downstream consumer in the validate path.

## Evidence

- `~/fp-beds/lt-evidence/cp67/RESULT.md` (BUG-LIVE-2; L-67-B FAIL)
- `~/fp-beds/lt-evidence/cp67/l67b-run19151-page_test.go.tampered` — the tampered locked file that survived to completion
- `~/fp-beds/lt-evidence/cp67/l67b-run19151-siglock-diagnosis.txt` — diagnosis notes incl. tamper non-propagation

## Severity

- `high` — evidence-tamper detection exists but is silently dropped at the gate that should enforce it; locked-file integrity is unverified at validate.

## Completion Notes (implemented 2026-09-23, CA-919)

- runValidateWithOracleIfPossible forces ExitCode=1 + tamper detail when oracle.Tampered non-empty. Test: bug387_validate_tamper_test.go.
