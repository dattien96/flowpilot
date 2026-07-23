# CA-405: Fix 2 approval tests broken by BUG-299's force-YOLO policy

## Summary

While auditing why a broad `go test ./internal/runner/...` sweep showed 18
failures unrelated to CP-51, traced 2 of them
(`TestApprovalDenyThenApprove`, `TestApprovalExpiry`) to a real interaction
with `4c2a254` ([BugFix][yolo-policy] force Flow YOLO true on create and
follow-up BUG-299, authored earlier the same day) -- not a bug in BUG-299
itself, but a shared test fixture that unintentionally now trips its new
rule. Operator confirmed: YOLO=true for Flow mode is an intentional product
requirement, so the outdated tests should be updated, not the policy.

## Root cause

BUG-299 added `shouldForceFlowYolo(runKind, workflowID, flowEngineDriven)`
(`yolo_resolver.go`), which forces YOLO=true whenever `workflowID` is
non-empty (among other conditions) -- correct for real Workflow/Flow runs.

The shared test helper `startRun()` (`interactive_service_test.go:65-79`,
used by 12+ tests in this file and more across 6 other test files) hardcodes
`WorkflowID: "wf-feature"` as a generic placeholder to satisfy the API's
required field -- it never meant to represent a real workflow launch. BUG-299
can't tell the difference, so every test using `startRun()` now gets YOLO
forced on.

`TestApprovalDenyThenApprove` and `TestApprovalExpiry` specifically depend on
YOLO staying off: they send a turn that calls a tool, expecting the runner to
surface a **pending approval** card to deny/approve/expire. With YOLO forced
true, the tool auto-approves instantly, no approval card ever appears, and
the tests time out waiting for one.

## Fix

Added `startNormalChatRun()` (`interactive_service_test.go`, right after
`startRun`) -- starts a run via `ChatMode: "normal_chat"` (which tags
`RunKind="chat"`, exempt from `shouldForceFlowYolo`) instead of a
`WorkflowID`. Updated only the 2 affected tests' 3 `startRun(...)` call sites
to use it; `startRun()` itself and its 12+ other callers are untouched.

A real chat-mode run also exercises live gate-observation
(`observe git diff`), which can still hold a file handle in the run's
workspace directory when the test function returns -- `t.TempDir()`'s own
cleanup then fails outright on Windows (open handles block deletion there,
unlike Unix), which was a new failure `startRun`'s workflow-mode path never
triggered. `startNormalChatRun` manages its own workspace dir with a
tolerant, retrying `os.RemoveAll` in `t.Cleanup` instead of relying on
`t.TempDir()`'s strict one-shot removal -- a test-hygiene fix, not a
correctness one.

## additive-tests-only compliance

Operator explicitly authorized updating these 2 outdated tests (BUG-299's
policy is an intentional requirement). Only their **setup** (which helper
starts the run) changed; every existing assertion in both tests is untouched.
One new helper function added; `startRun()` and all its other callers
untouched.

## Verification

- `go build ./...`, `go vet ./internal/runner/`: clean.
- `TestApprovalDenyThenApprove` + `TestApprovalExpiry`: pass, confirmed
  clean across 5 repeated `-count=1` runs (checking for flakiness from the
  async gate-observation path).
- Full `go test ./internal/runner/...` sweep: both tests no longer appear in
  the failure list. Remaining ~16-17 failures are unrelated (Windows
  HOME/USERPROFILE env-var handling, real `codex` CLI dependency, previously
  pre-existing chat-sync/Grok-account-slot failures, and a shifting set of
  test-order-dependent flakes -- none touch CP-51/dispatch code and none are
  affected by this change).

## Follow-up (not addressed here, out of scope)

The full-package sweep shows the specific set of "extra" failures shifts
between runs (e.g. `TestGeminiAdapterPromptPrepAndEnv` and
`TestWorkflowDrivenQuestionAnswerPersistsRunningStatus` appeared only after
these 2 were fixed, while `TestStartTurnGrokSameAccountFollowUpUsesPromotedRealID`
disappeared) -- suggesting broader test-order/global-state pollution in this
package beyond the two fixed here. Worth its own dedicated investigation.

# ---8<--- flowpilot:change-ledger
feature_key: yolo-policy
source_doc_id: BUG-299
change_type: test
summary: Fixed 2 approval tests that broke when BUG-299's force-YOLO policy started tripping on a shared test fixture's placeholder WorkflowID; added a chat-mode run helper instead of touching the shared fixture.
# --->8---
