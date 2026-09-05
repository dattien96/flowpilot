# CA-751 — Task-325 follow-up: /continue carries human feedback text (TUI)

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-325
change_type: feature
summary: TUI /continue accepts trailing text as human feedback for parked flows (plan_approval re-entry); bare form and Retry chip keep legacy body
# --->8---

## Gap (found during Task-325 live-verify, run-577686)

Task-325 T-4 claimed both clients surface Approve/Feedback/Stop. True for Desktop (FlowAwaitingUserCard textarea) but false for TUI: `cmdContinueFlow` posted hardcoded `feedback:"continue"` — an operator parked on cap/plan_approval had nowhere to type the feedback the engine already accepts on `POST .../agent-loop/continue {"feedback"}`.

## Change (TUI-only)

- `tui/client/client.go`: `ContinueFlowWithFeedback(ctx, runID, feedback)`; `ContinueFlow` delegates with `"continue"` (unchanged wire behavior).
- `tui/app/app.go`: `cmdContinueFlowWithFeedback`; `/continue` joins trailing args via `continueFeedbackFromArgs` (empty → legacy path); `[Retry]` chip path untouched.
- Tests (new `continue_feedback_test.go`): arg-join table; `/continue <text>` POSTs the text (httptest capture, claude/codex/grok matrix); bare `/continue` keeps `"continue"`.

## Verification

- 3 new tests PASS; full `tui/...` green; zero old-test edits; gofmt clean on new lines.
- Note: picking this up needs a TUI rebuild/restart; a parked run's LoopState is durable so the park survives (children re-resolve via fallbacks).
