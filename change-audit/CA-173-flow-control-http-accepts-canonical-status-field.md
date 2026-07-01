# CA-173: /flow-control HTTP Handler Accepts the Canonical `status` Field (BUG-NOTE-CP42 #32)

## Scope

Verified and fixed a P2 issue from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`: `POST /client/workflow-runs/{runId}/flow-control` rejected a request using `ReviewOutcomeInput`'s own canonical wire field.

## The bug

`ReviewOutcomeInput`'s Go struct tags its field `Status string \`json:"status"\`` — `"status"` is the canonical wire key; `"outcome"` is only a legacy/board alias `parseReviewOutcomeInput` also accepts. `handleSubmitFlowControl` (`interactive_handlers.go`) decided which parser to use purely by checking whether the request body had a literal `"outcome"` key. A caller sending the canonical shape, `{"status": "approved"}`, has no `"outcome"` key, so the handler routed it to the *raw* `FlowControlInput` parser instead — which only recognizes `status` values `continue`/`done`/`escalate` — and rejected `"approved"` outright with a 400.

## Fix

Changed the routing condition from `hasOutcome` alone to `hasOutcome || reviewOutcomeStatuses[statusVal]`, checking `body["status"]` against the existing `reviewOutcomeStatuses` map (`agent_orchestrator.go`: `approved`/`changes_requested`/`blocked`). These values never overlap with the raw `FlowControlInput` status values, so the two shapes remain unambiguous. `parseReviewOutcomeInput` itself needed no changes — it already correctly checked `"status"` before falling back to `"outcome"`.

## Verification

- New test `TestSubmitFlowControlHTTPAcceptsCanonicalStatusField` (`interactive_service_test.go`): POSTs `{"status": "approved"}` and asserts it succeeds (200) and maps to `done`, mirroring the existing `outcome`-alias test.
- Existing `TestSubmitFlowControlHTTPAcceptsOutcomeAlias`/`...OutcomeChangesRequested` (the `outcome`-keyed shape) pass unchanged.
- `go build ./...` clean.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: route /flow-control's ReviewOutcomeInput parsing on either the outcome alias key or the canonical status field's value, instead of only the literal outcome key's presence
# --->8---
