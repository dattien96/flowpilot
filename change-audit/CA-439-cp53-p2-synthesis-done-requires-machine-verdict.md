# CA-439 — CP-53 P-2 Synthesis Done Requires Machine Verdict (Task-274)

## Scope

Retroactive change-audit for **Task-274 / CP-53 P-2**: hub `approved→done` on flows with `acceptance_nodes` including synthesis requires recorded reviewer **machine verdicts** via `submit_review_outcome`; prose alone cannot close the loop.

## Prior CA (will not undo)

- **CA-428**: canonical mutation stays at terminal flow acceptance — this task gates *when* done is allowed, not where finalize runs.
- **CA-431**: migrated review-loop topology and context render unchanged.
- **CA-417**: cohort rejoin / hub reinvoke paths preserved; reviewer submit is record-only until hub acts.

## Changes

- `runner/review_done_verdict.go` (new): `flowRequiresSynthesisMachineVerdict`, cohort label helpers, `synthesisDoneVerdictError`.
- `runner/interactive_service.go`: `applyFlowControl` blocks hub `approved→done` when synthesis acceptance active and reviewer verdicts missing or not approved; tracks `pendingReviewVerdictByLabel` / `lastReviewCohortVerdicts`.
- `runner/flow_executor.go`: seeds `activeFlowAcceptanceNodes` from flow definition.
- `turnBridge` / `SubmitFlowControl`: reviewer cohort members record verdict without advancing flow.
- `agents/reviewer.md`: instruct machine verdict tool call.
- Tests (additive): `runner/cp53_review_done_verdict_test.go` — blocked without verdict, fail reviewer, pass, continue unaffected, normal chat unaffected, reviewer record-only, **Claude/Codex/Grok matrix** via fake adapters.

## Provider impact

**Shared flow runtime (Case 2)** — verdict gate is hub-side; verified with parameterized matrix:

| Provider | Evidence |
|----------|----------|
| Claude | `TestCP53ReviewDoneVerdictProviderMatrix/claude` |
| Codex | same subtest + default unit tests |
| Grok | same subtest |

Live adapter E2E on all three not run in this task — bridge/unit matrix is the contract proof.

## Backward compatibility

Raw `status=done` (not `approved→done`) still allowed for legacy E2E/manual spawn paths without `activeFlowAcceptanceNodes`.

## Verification

```bash
go test ./internal/runner/ -run 'TestCP53ReviewDoneVerdict|TestFlowRequiresSynthesisMachineVerdict|TestSubmitFlowControl' -count=1
```

## Out of scope / residual

- `TestE2EReviewLoopApprovedPath` observed hanging on Windows during safe-fix audit — investigate separately (uses raw `status=done`, should remain compatible).
- Reviewer model/effort asymmetry (CP-53 D-2) not configured in this commit.

## Commits

- `84fb557` — implementation

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-274
change_type: feature
summary: Task-274 P-2 hub approved-to-done requires reviewer machine verdicts on synthesis acceptance flows with Claude Codex Grok bridge matrix tests
# --->8---
