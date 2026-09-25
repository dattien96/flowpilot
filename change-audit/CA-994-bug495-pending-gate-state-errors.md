# CA-994 — BUG-495: resume pending-gate reads fail closed

## What changed

`apps/local-runner/internal/runner/interactive_resume.go`:

- New `pendingGateStates(runID)` helper reads `ApprovalHistoryReader` /
  `QuestionHistoryReader` and propagates errors instead of the previous
  `err == nil` gating that produced empty pending lists on a store fault.
- `reconstructRunInternal` aborts with `502 gate_state_unavailable` when a
  gate sidecar read fails — a durably waiting approval/question node can
  no longer be promoted to not-waiting on resume.
- `childPendingGateNodeIDs`: per-child `ListApprovalsByRun` /
  `ListQuestionsByRun` errors now propagate (same contract as the session
  index read fixed in BUG-491).

## Invariant

Store-read errors must not become empty authoritative views. A gate whose
answer state cannot be proven stays waiting — the run aborts resume
instead of silently advancing.

## Tests

`bug495_pending_gate_state_swallow_test.go`: faulting approval store →
error; faulting question store → error; healthy store → pending states
reach the keep-waiting merge unchanged.
