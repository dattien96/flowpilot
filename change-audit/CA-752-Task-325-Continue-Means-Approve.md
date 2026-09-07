# CA-752 — Task-325 follow-up: legacy "continue" counts as approve on plan_approval parks

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-325
change_type: bugfix
summary: plan_approval resume treats empty feedback and the legacy continue token as approve so the TUI Retry chip and bare continue can approve; genuine feedback still re-enters the writer
# --->8---

## Gap (found during Task-325 live-verify, run-584646)

`resumePlanApproval` treated only empty feedback as approve. But both TUI approve surfaces send the legacy `"continue"` token (Retry chip via `cmdContinueFlow`, bare `/continue` via `ContinueFlow`) — so approve was unreachable without curl. (Desktop textarea sends `""` and already worked.)

## Change (runner, 2 files)

- `runner/plan_approval_park.go`: approve iff trimmed feedback is empty OR case-insensitive `"continue"`. A literal human "continue" revision note is indistinguishable and intentionally counts as approve (recoverable either way; documented on the function).
- `runner/task325_plan_approval_park_test.go`: approve matrix `""` / `"continue"` / `"  Continue  "` — all advance to freeze with the writer untouched.

## Verification

- 11 Task-325 tests PASS; related old suites (`run201295`, `applyFlowControl`, resume-with-feedback) green; zero old-test edits.
- Live: run-584646 park trigger verified end-to-end (churned plan round 2 approve → `blocked/plan_approval`, `plan_synthesis WAITING_USER_APPROVAL`, freeze untouched, correct chips, GateReason names writer round + plan path via BUG-357 record).
