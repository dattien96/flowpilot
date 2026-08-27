# CA-654: TUI blocked bar separate action chips and descriptions per line

## What

UI cleanup for the TUI blocked decision bar (`renderBlockedBar`) when a workflow is parked awaiting user decision (escalate, cap, or scope drift):

- `apps/local-runner/internal/tui/app/step_runtime.go`: render each action chip and description on its own line (`  [Retry] - run again with old scope`, `  [Stop] - end flow`, and conditionally `  [Allow] - continue with new scope (match code changed)`).
- Replaces previous multi-column token layout that placed descriptions on a separate shared row.

## Tests

- `go test ./internal/tui/app/... -count=1` PASS
- `just chat-test` PASS

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-309
change_type: refactor
summary: format blocked action chips and descriptions onto separate lines in TUI
# --->8---
