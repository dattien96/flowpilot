# CA-1169 — run-2062497 D6+D7: adjudication redrive routing and signature renegotiation

## What changed

`apps/local-runner/internal/runner/review_done_verdict.go`:

- Continue-side deficient-member recovery now also fires when the park
  reason names a deficient cohort member (e.g. "SPEC_ALIGN: blocked") even
  if the reason is not a recognized review-verdict gate phrase. Live: the
  synthesis hub wrote "Cohort split needs human adjudication —
  REVIEWER: approved, SPEC_ALIGN: blocked"; `isReviewVerdictGateReason`
  rejected the prose, so Continue re-invoked the hub against the stale
  verdict map and reparked identically. Reasons naming no member keep the
  generic resume (CA-1098 boundary preserved).

`apps/local-runner/internal/runner/coder_outcome.go` /
`interactive_service.go` (scaffold lock):

- `recordScaffoldArtifactsLock` adds a `prodSignatures == ""` fallback:
  when the scaffold turn produced test files but no production-symbol
  signature stream (prod stubs already existed), the contract still pins a
  `signature_hash` over the union of written + declared paths. Live: every
  run-2062497 contract froze with `signature_hash=""` →
  `isSignatureLockedCoderChild` false → no `submit_review_outcome` tool was
  offered → a coder's correct batched `renegotiate_signatures` could not be
  recorded and the turn retried instead of routing to
  `synthesis_negotiation`.
- `pendingBatchSignatureByStep` read-side now flattens and sorts all
  buffered keys instead of indexing by the current `parent.stepID` — the
  step re-stamps per turn, so a batch submitted before a park/resume was
  orphaned under the old key.

## Invariant

An adjudication park that identifies a deficient member redrives the member
(not the hub that already reported), a locked-test coder always has its
typed outcome tool, and a signature batch survives step-id re-stamping.

## Tests

`ca1098_missing_verdict_resume_test.go`
(`TestRun2062497_SplitAdjudicationContinueRedrivesDeficientMember`,
`TestRun2062497_AdjudicationReasonWithoutVerdictKeepsGenericResume`),
`scaffold_lock_test.go`
(`TestRun2062497_ScaffoldTestOnlyTurnStillPinsSignatureHash`),
`cp67_coder_transport_test.go`
(`TestRun2062497_CoderBatchSurvivesStepIDRestamp`).
