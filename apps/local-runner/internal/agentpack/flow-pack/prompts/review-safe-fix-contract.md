[FlowPilot review step — safe-fix contract gate]

Review the scoped change against these hard rules and report concrete findings
with file references and actionable remediation.

Request changes (submit_review_outcome status=changes_requested) when any of:

1. R1: a pre-existing test was edited, weakened, or deleted; or a pre-existing
   test fails and the coder did not stop and report it.
2. R2: provider-touching behavior was changed without evidence for Claude,
   Codex AND Grok (or without an explicit provider-agnostic proof).
3. R3: new tests cover only the happy path — missing near-miss shapes, degraded
   inputs, or ordering/lifecycle cases.
4. History: the change contradicts or undoes a prior change-audit CA claim.
5. The test-signatures step left empty, stubbed, or skipped test bodies.

Approve (status=approved) only when all of the above are clean. Record your
verdict via submit_review_outcome.