# CA-856 — CP-62 P-2 follow-up: per-AC verdict coverage wired into the submit path

# ---8<--- flowpilot:change-ledger
feature_key: zcode-parity
source_doc_id: Task-344
change_type: task
summary: wire Task-338's ValidateReviewOutcomeVerdicts/ExtractACIDs into the live submit path — deterministic governing-artifact resolution (vibe short task name located under requirements/, own vibe plan entry for the inline-hub submitter, read_only node INPUT pathTemplate glob-newest, sibling required OUTPUT pathTemplate glob for binding-less reviewers) and one enforcement point at the top of turnBridge.SubmitFlowControl covering all three provider faces; verdict_only owners never AC-covered, hub/partial-only rule keeps empty submissions byte-identical
# --->8---

## Why

Task-338 shipped the pure coverage checks but no production call site invoked them — a reviewer could still rubber-stamp with verdicts missing for ACs in the governing task doc (the documented CP-62 P-2 follow-up (a)).

## Change

- `runner/review_ac_coverage.go` (new): `validateReviewACCoverage` + `expectedReviewACs` (cached per child run: `expectedACsCache/expectedACsResolved` fields on `interactiveRun`) + deterministic artifact sources in order: (1) `rs.vibeTaskName` — runtime value is `shortTaskName`, so a bare file name is located under `<ws>/requirements` newest-match; (2) own `vibeTaskPlan[vibeSprintIndex-1]` entry (root/inline-hub vibe submitters); (3) submitting node's INPUT `file_artifact.v1` `config.pathTemplate` — `{{idx}}`→`\d+`, `{{slug}}`→`[a-z0-9-]+`, newest mtime wins; (4) sibling required OUTPUT pathTemplate bindings (harness reviewer has no binding of its own; the writer resolves the concrete name at authoring time, CP-58 Task-307).
- `runner/interactive_service.go`: enforcement inserted at the top of `turnBridge.SubmitFlowControl` when `in.viaReviewOutcome` — the error result IS the in-turn reprompt; single funnel covers claude MCP, codex, and grok bridge paths (parity by construction).
- Enforcement matrix: `verdict_only` (owner debate) never AC-covered (Q-3/Task-340); `read_only` faces enforce even with zero verdict rows (rubber-stamp guard); posture-"" submitters (inline vibe hub) enforce only when verdict rows were submitted (partial rows rejected; empty submissions keep legacy byte-identity); unresolvable artifact → no-op (Task-338 fault-tolerant fallback).

## Tests

`runner/review_ac_coverage_test.go` (9, additive): vibe doc missing-AC rejection + full-set pass; INPUT template newest-file-wins (mtime-ordered Task-1/Task-2); sibling OUTPUT template for binding-less reviewer; owner verdict_only never enforced; no-doc passthrough; raw flow_control skipped; vibe root hub empty-passthrough/partial-reject; bridge-level rejection precedes processing and a full resubmit succeeds; cache survives doc deletion mid-review.

## R1 / regression evidence

Full runner suite on this machine is noisy on BOTH sides: base worktree (f6634215, no changes) fails 22 tests; branch fails 23 with the same flake pool plus `TestCA791_SlicerAfterIngestJoinStartsSprint` (fails 1/10 on base too — pre-existing flake) and two tests that pass 3/3 in isolation (`TestWorkflowDrivenQuestionAnswerPersistsRunningStatus`, `TestStartTurnGrokCrossAccountLegacyThreadPromotesCopiesAndLoads`). Targeted suites (coverage 9/9, CP-62 surface: verdict/decision/isolation/profile/catalog/handoff/conventions/vibe gate) green. Pre-existing machine-dependent failures unchanged: TestBUG327 (hang), TestTask330, TestResumeRunEchoesChatIdentity, TestFirebaseToolsMcpAdapterFetchEndToEnd.

## Providers

Case 1 provider-agnostic — enforcement sits on the shared TurnBridge submit funnel; no adapter edits, no provider-specific branches.

## Prior claims intact

Task-338 pure functions untouched (this is their first live caller); CP-61 hub-done verdict gate untouched; cohort verdict recording untouched (rejection happens before `recordReviewCohortMemberVerdict`); board endpoint (human face) deliberately out of scope; Task-342 handoff capture path unchanged.
