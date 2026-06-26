# CA-058: History Replay Restores User Prompts

## Summary

- Fixed desktop history replay losing user prompts when reopening a run from the history popover.
- Added the resolved bugfix record at `requirements/09-BugFix/done/BUG-046-Desktop-History-Replay-Loses-User-Prompts.md`.
- Persisted the submitted prompt on the runner `turn_started` event so replay streams contain the same user prompt boundary as live sends.
- Updated desktop replay folding to render prompt bubbles from replayed `turn_started.prompt` events while avoiding duplicate prompt bubbles during live sends.
- Updated the mock desktop runner to emit the same `turn_started.prompt` shape as the local runner.

## Changed Files

- `apps/local-runner/internal/runner/interactive_service.go`
- `apps/local-runner/internal/runner/interactive_service_test.go`
- `apps/desktop-flowpilot/src/types/contract.ts`
- `apps/desktop-flowpilot/src/client/MockRunnerClient.ts`
- `apps/desktop-flowpilot/src/state/store.ts`

## Verification

- `go test ./internal/runner -run TestNormalTurnPersistsWithSeq -count=1` in `apps/local-runner`
- `npm run typecheck` in `apps/desktop-flowpilot`
- `npm run build` in `apps/desktop-flowpilot`

## Notes

- `go test ./internal/runner -count=1` was also run, but it fails on pre-existing environment/config requirements unrelated to this bug: Google Drive proxy auth setup and missing `powershell` on this machine.
- The installed GitNexus CLI does not expose `detect_changes`; scope was checked with `git status --short` and `git diff`.

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: BUG-046
change_type: feature
summary: History Replay Restores User Prompts
# --->8---
