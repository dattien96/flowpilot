# BUG-386: r-signature-lock never arms — frozenContractForRun returns the scaffold's hashless record

## Metadata

- Document ID: `BUG-386`
- Title: `r-signature-lock never arms — frozenContractForRun returns the scaffold's hashless record`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-67-Test-Steps](../../07-Coding-Plan/todo/CP-67-Test-Steps.md)
- Feature Keys: `contract-first-tdd`

## AI Quick View

### Summary

- Live run-19151 (vibe-sprint, pageutil): the coder shipped `func PageBounds(total, page, size int) (first, last int)` against a locked signature of `(start, end int)` plus an extra `PageCount` helper — declaration drift — and the run completed with **zero** `r-signature-lock` violations.
- `InteractiveService.frozenContractForRun` iterates code-writer nodes in topology order and returns the **first** active frozen record; the scaffold writer (`tdd`/`test_signatures`) precedes the coder and its record has no `SignatureHash` → `coderSignaturesLocked` is never true → `r-signature-lock` is absent from the coder's rule set.
- Same root cause makes `isSignatureLockedCoderChild` return false → `renegotiate_signatures` tool is never advertised to the coder → the entire CP-67 renegotiation loop is unreachable live (L-67-C blocked; only transport-level HTTP 400 schema rejection was verifiable).

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** Signature-locked coder run ships renamed return vars + an extra exported function with no gate violation; run completes clean. The coder child is never offered `submit_coder_outcome`/`renegotiate_signatures` even though its prompt carries the batch-renegotiation instructions.
- **Expected:** Once the coder's frozen contract carries a `SignatureHash` (v2 `f9beeade7c1f2880`), `r-signature-lock` is armed for the coder step and the renegotiation tool is advertised so signature changes route through `synthesis_negotiation`.
- **Actual:** `frozenContractForRun` returns the scaffold writer's hashless record → `SignatureHash == ""` → `coderSignaturesLocked` false → rule never armed; `isSignatureLockedCoderChild` false → tool never advertised (`interactive_service.go:7408`). Signature violations ship silently.
- **Impact:** The CP-67 signature-lock guarantee is void in every multi-record flow (scaffold + coder). Declaration drift ships undetected, and the batch-renegotiation path (transport verified via 3× HTTP 400 on run-15864) can never be exercised end-to-end live — `synthesis_negotiation` hub routing and task-harness↔vibe parity are unverifiable.

## Reproduction

1. Run a Contract-First Scaffold TDD flow (vibe-sprint) where the scaffold node freezes first and the coder node later freezes a hash-bearing v2 — e.g. CP-67 L-67-B pageutil bed.
2. Let the coder drift the locked signature (rename return vars, add `PageCount`).
3. Observe: gate emits no `r-signature-lock` violation; session tool list lacks `submit_coder_outcome`; run completes.
- runIds: `run-19151` (drift shipped), `run-15864` (transport-only verification: 3× HTTP 400 `invalid_outcome` on malformed batches — no live valid batch possible).

## Root cause

- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:2226` — `frozenContractForRun` iterates `flowAgentCodeWriterNodes(nodes)` in topology order and returns the first active frozen record. The scaffold writer precedes the coder; its record has no `SignatureHash`. The `ListForRun` fallback scan has the same ordering problem (any active record with non-empty `CoderStepID` — scaffold included).
- `apps/local-runner/internal/runner/coder_outcome.go:108` — `isSignatureLockedCoderChild` reads the same record → `SignatureHash == ""` → false → tool advertisement gate at `interactive_service.go:7408` never fires.
- Why unit tests miss it: `TestIsSignatureLockedCoderChild` freezes the contract directly on `"implement"` — single-record fixture; the ordering bug cannot manifest.

## Evidence

- `~/fp-beds/lt-evidence/cp67/RESULT.md` (BUG-LIVE-1, BUG-LIVE-3; L-67-B FAIL, L-67-C FAIL)
- `~/fp-beds/lt-evidence/cp67/l67b-run19151-contract-v2.json` — coder v2 `f9beeade7c1f2880` (hash-bearing record that was never selected)
- `~/fp-beds/lt-evidence/cp67/l67b-run19151-frozen-contracts.ndjson` — tdd v1 hashless, coder v1 hashless, coder v2 hash-bearing
- `~/fp-beds/lt-evidence/cp67/l67b-run19151-page.go.drifted` — shipped `PageBounds`/`PageCount` drift
- `~/fp-beds/lt-evidence/cp67/l67b-run19151-siglock-diagnosis.txt` — full read-only diagnosis
- `~/fp-beds/lt-evidence/cp67/l67c-batch400-evidence.txt` — 3× HTTP 400 transport enforcement on run-15864

## Severity

- `critical` — the flagship CP-67 coder-side guarantee never engages; signature drift ships silently and the renegotiation loop is dead code in live runs.

## Completion Notes (implemented 2026-09-23, CA-919b)

- frozenContractForRun now prefers the SignatureHash-bearing record (topology scan + ListForRun fallback); r-signature-lock arms and renegotiate_signatures advertises. Test: bug386_frozen_contract_preference_test.go.
