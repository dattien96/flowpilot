# CA-545 - parallel approval/question cards overwrite each other; dropped card hangs the run

## Problem

A single turn can fan out several approval or question cards in parallel — one
tool approval while another tool (or another agent) asks a separate question
(BUG-157/BUG-158, CA-195). The TUI kept exactly one `approval` and one
`question` pointer; each new `permission_required`/`user_question_required`
event overwrote the previous card. The dropped card stayed pending server-side
(no decision/answer ever submitted), leaving the run parked on a gate with no
UI to resolve it. Desktop already queues cards (`pendingApprovals`/
`pendingQuestions` in store.ts); the TUI did not.

## Fix (TUI-only; runner + desktop unchanged)

G1 of the G1/G2/G3 interactive-card parity plan. Head pointer kept as a
compatibility shim so every legacy `m.approval != nil` check keeps working.

- `model.go`: new `approvals []ApprovalState` / `questions []QuestionState`
  queues alongside the existing `approval`/`question` head pointers.
- `chat_pending_queue.go` (new): `pushApproval`/`pushQuestion` (append + dedup
  by ID, point the head at the first pending card), `removeApproval`/
  `removeQuestion` (drop by ID or all when ID empty), `sync*Head` (re-derive the
  head from the queue as a copy so slice mutations never corrupt it),
  `pendingApprovalShown`/`pendingQuestionShown`, `clearPendingDecisions`,
  `resolveAllApprovals` (batch-resolve every queued card).
- `app.go` `ApprovalResolvedMsg`/`QuestionResolvedMsg`: remove the resolved card
  from the queue, advance the head, and stay ConnWaiting with an "N more
  pending" line while cards remain.
- `app.go` `permission_required`/`user_question_required`: push (deduped),
  print the waiting line only for a new card.
- `app.go` `/approve-all` `/deny-all` slash commands (resolve-all) and the
  approval input bar shows `approval 1/N` + `Approve all`/`Deny all` chips when
  more than one card is queued; question bar shows `(1/N)`.
- `mouse.go`: `hitApprovalChrome` maps the bulk chips, dispatch resolves all.
- `app.go`/`history.go` reset + hydrate paths route through the queue helpers so
  the head pointer and slice never drift.

## Files

- `apps/local-runner/internal/tui/app/app.go`
- `apps/local-runner/internal/tui/app/model.go`
- `apps/local-runner/internal/tui/app/mouse.go`
- `apps/local-runner/internal/tui/app/history.go`
- `apps/local-runner/internal/tui/app/chat_pending_queue.go` (new)
- `apps/local-runner/internal/tui/app/ca195_parallel_approval_queue_test.go`
  (new)

## Tests

Additive: `ca195_parallel_approval_queue_test.go` covers two parallel
`permission_required` events queue without orphan (both surface in the
transcript, composer blocked), same-ID re-emit dedups to one card, resolving the
head advances to the next card and stays ConnWaiting, `resolveAllApprovals`
fires one POST per card (mock runner) and empties the queue after
`ApprovalResolvedMsg`, single card preserves the old UX (clears, "Approved.",
no queue mention), two `user_question_required` cards advance after the first
answer, and the render bars show `approval 1/N` + bulk chips / `(1/N)` with
clickable `approve-all`/`deny-all` targets.

Pre-existing approval/question tests pass unchanged (`run97624_...`,
`chat_question_select_test.go`, `chat_ux_actions_test.go`, `task291_...`,
`tui_approval_waiting_copy_test.go`, `cp56_signatures_test.go`).

## Verification

- `go test ./internal/tui/app/ -count=1` green except the two pre-existing
  environmental `TestCmdFocusAgent_*` network failures (confirmed unchanged).
- `go test ./internal/runner/ -count=1` and `go test ./internal/tui/client/`
  green. (One `TestFinalizerHookSurfacesArtifacts` flake under the combined
  run passes in isolation and on re-run; not related to this change — diff is
  TUI-app-only.)
- gofmt clean on all edited/new files (LF-normalized temp copies); CRLF
  preserved; `go vet ./internal/tui/app/` clean.
- Provider-agnostic (cross-provider-parity Case 1): the touched handlers and
  helpers take no `providerKey` and never branch on one; the pre-existing
  `TestApplyPendingFromSnapshot_WaitingApproval` already loops grok/claude/codex
  and passes.

## Change Ledger

```json
{
  "flowpilot:change-ledger": {
    "source_doc_id": "CA-545",
    "change_type": "bugfix",
    "feature_key": "cli-tui",
    "impacted_areas": ["tui-gates", "tui-approval", "tui-question", "tui-composer"],
    "upstream_docs": ["SS-07", "CP-05", "CA-195"],
    "status": "verified"
  }
}
```