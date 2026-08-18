# CA-546 - TUI approval card drops runner decision details and cannot "don't ask again"

## Problem

Runner `provider_event.go` already sends rich approval data — `Details`
(`ApprovalDetails{Command, Cwd, Reason, Kind, Decisions}`) and, on replay, a
`Decision` string — and the runner's `SubmitApproval` accepts `decision`,
`remember`, and `forever`. Desktop renders decision-specific buttons
(`ApprovalCard.tsx`). The TUI kept ignoring all of it: the card only showed
`Approve`/`Deny`, the reason/command line was missing, and there was no way to
persist a per-project "don't ask again" approval (BUG-246).

## Fix (TUI + shared client only; runner + desktop unchanged)

G2 of the G1/G2/G3 interactive-card parity plan. No provider branch; the card
renders whatever the runner sends.

- `internal/tui/client/client.go`: `ProviderEvent` gains `Details
  *ApprovalDetails` and `Decision string`; new `ApprovalDetails{Command, Cwd,
  Reason, Kind, Decisions []ApprovalDecisionOption}` and
  `ApprovalDecisionOption{Value, Label}` types.
- `model.go`: `ApprovalState` gains `Command`, `Cwd`, `Reason`, `Kind` strings
  and `Decisions []client.ApprovalDecisionOption`.
- `app.go` `permission_required`: replay (`Decision != ""`) renders a read-only
  `[APPROVAL] {id} already resolved: {decision}` line, never mounts a card and
  never parks the composer; otherwise the card is built from `ev.Details` and
  `formatApprovalWaitingLineDetailed` prints `kind: command — reason`. The
  legacy `formatApprovalWaitingLine` is untouched for detail-less cards.
- `app.go`: `submitPendingApprovalDecision`, `submitPendingApprovalRemember`,
  `cmdApproveWithRemember`; `/approve forever` and `/deny forever` slash
  commands; the bar uses `approvalDecisionChips` (decision labels, or
  `Approve`/`Deny` fallback) plus `Approve forever` only when
  `approvalRememberable` (kind `exec`, non-compound command, and an approve
  decision exists or decisions are empty). `deny` never persists
  (`remember=false`).
- `mouse.go`: `hitApprovalChrome` is now a method; adds `Approve forever` and a
  per-decision `adec:<value>` token; dispatch resolves them.
- `chat_pending_queue.go`: `approvalCommandOperators` (mirrors desktop
  `ApprovalCard.tsx` COMMAND_OPERATORS), `isCompoundCommand`,
  `approvalRememberable`, `approvalDecisionChips`, `approvalStateFromInfo`
  (hydrate typed fields from a snapshot `Details map[string]any`).
- `history.go`: hydrate through `approvalStateFromInfo` + the detailed line.

## Files

- `apps/local-runner/internal/tui/app/app.go`
- `apps/local-runner/internal/tui/app/model.go`
- `apps/local-runner/internal/tui/app/mouse.go`
- `apps/local-runner/internal/tui/app/history.go`
- `apps/local-runner/internal/tui/app/chat_input_cursor.go`
- `apps/local-runner/internal/tui/app/chat_pending_queue.go`
- `apps/local-runner/internal/tui/client/client.go`
- `apps/local-runner/internal/tui/app/ca246_approval_details_remember_test.go`
  (new)

## Tests

Additive: `ca246_approval_details_remember_test.go` covers details populating
the card (kind/command/decision chips/forever chip), replay `Decision`
rendering read-only with no card and no composer park, the remember-eligibility
rule (exec + non-compound + approve decision), `/approve forever` posting
`remember=true&forever=true`, `/deny forever` never persisting, default chips
when the runner sends no decisions, and a click on a decision label submitting
the runner's exact value to the mock runner (`approve_for_session`).

Pre-existing approval/question tests pass unchanged (`ca195_...`,
`run97624_...`, `chat_question_select_test.go`, `chat_ux_actions_test.go`,
`task291_...`, `tui_approval_waiting_copy_test.go`, `cp56_signatures_test.go`).

## Verification

- `go test ./internal/tui/app/ ./internal/tui/client/ -count=1` green except the
  two pre-existing environmental `TestCmdFocusAgent_*` network failures in
  `tui/app` (confirmed unchanged).
- gofmt clean on all edited/new files (LF-normalized temp copies); CRLF
  preserved; `go vet` clean.
- Provider-agnostic (cross-provider-parity Case 1): no `providerKey` branch
  anywhere; the card renders the runner's `Details` verbatim and the pre-existing
  provider-loop test `TestApplyPendingFromSnapshot_WaitingApproval` still passes.

## Change Ledger

```json
{
  "flowpilot:change-ledger": {
    "source_doc_id": "CA-546",
    "change_type": "bugfix",
    "feature_key": "cli-tui",
    "impacted_areas": ["tui-approval", "tui-gates", "tui-composer", "tui-client"],
    "upstream_docs": ["SS-07", "CP-05", "BUG-246", "CA-545"],
    "status": "verified"
  }
}
```
