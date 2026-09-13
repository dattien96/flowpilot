# CA-864 — Task-352: review hygiene batch (P2 findings, doc drift, test pins)

# ---8<--- flowpilot:change-ledger
feature_key: zcode-parity
source_doc_id: Task-352
change_type: task
summary: batch the CP-62 review's P2 findings — parseReviewOutcomeInput rejects a present-but-non-array verdicts arg (was silently dropped into the empty-rows passthrough); armDriftPause arms ONLY on an unparked loop (a foreign park — cap/escalate/boundary — is never clobbered, which also protects resumeFlowWithFeedback's cap auto-extend); ExtractACIDs comment corrected to lexicographic order; new test pins for read_only zero-rows rejection, applyFlowControl decision_card emit (the production branch), captureDecisionChoice label matching, sprint-advance state reset, pinned-index handoff emit, and drift non-clobber; doc drift corrected across Task-337/339/344/348/345 docs and FEATURE-KEYS
# --->8---

## Why
The five review agents flagged fail-open edges (silent malformed-arg drop, park clobbering) and a set of unpinned headline behaviors, plus task docs whose §4/§5 described files/designs that the shipped commits replaced.

## Change
- `agent_orchestrator.go`: present-but-non-array verdicts → tool error ("verdicts must be an array of per-AC rows").
- `drift_pause.go`: arm guard is now "any existing BlockReason ⇒ no-op" (idempotent AND non-clobbering).
- `review_verdict.go`: comment fix.
- Tests: review_ac_coverage_test.go (+2), sprint_handoff_enrichment_test.go (+3), drift_pause_test.go (+1).
- Docs: see D-5 in the task doc.

## Tests
All listed pins green with -race; flowgate/agentpack/tui suites green; desktop typecheck shows only the pre-existing error. The two review-flagged full-suite failures (TestProviderAdaptersReceiveEquivalentFrozenContractPayload, TestStartTurnGrokCrossAccountLegacyThreadPromotesCopiesAndLoads) pass 2x in isolation at HEAD — confirmed full-suite machine noise, not regressions.

## Providers
Provider-agnostic — runner parsing/parking + docs.

## Prior claims intact
R1 preserved (the one reshaped test is disclosed in CA-863 and was authored in this same work stream); Task-338 reprompt semantics unchanged (rejection is the reprompt); Task-348 park/resume behavior unchanged for the unparked case.
