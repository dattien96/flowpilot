# CA-910: TUI Scaffold Busy State and Runner Exit Telemetry

## Summary

A live React Native `/init all` stayed visually idle while the synchronous AI Scaffold request ran. The composer remained editable, so the operator could not tell whether work was active and could attempt chat sends into the same project. After 142 seconds the runner connection was forcibly closed; the TUI-spawned runner and Devin child both disappeared without `metadata.json`, `scaffold-status.json`, panic output, shutdown log, Windows crash report, or resource-exhaustion event.

The TUI now owns an explicit scaffold busy lifecycle. Starting the post-init scaffold marks live work, reuses the existing animated spinner/elapsed ticker, replaces the composer with `AI Scaffold running… (chat disabled)`, drops text/paste/send keys, and retains navigation plus Ctrl+C. Every terminal scaffold response, including transport errors, releases the lock and restores chat.

Runnerboot now writes the spawned runner PID and the result of `cmd.Wait()` to `.flowpilot/cli-runner.log`. The previous goroutine discarded the exit result, making an external termination, non-zero exit, or normal exit indistinguishable. The next reproduction will record the process-level exit evidence even when no Go panic reaches the log.

## Investigation Evidence

- TUI request started at 11:04:03 and failed at 11:06:25 with `wsarecv: An existing connection was forcibly closed by the remote host`.
- Port 4317 had no listener after the error; the TUI remained alive.
- Devin PID 27484 and the runner child were gone; the prompt run had `command.txt`, partial `stdout.txt`, empty `stderr.txt`, and no terminal metadata.
- No `/system/shutdown` log, signal cleanup line, panic, WER crash report, or Windows resource exhaustion event was present. Approximately 15 GB physical memory remained available.
- The exact termination source remains unproven because prior runnerboot code ignored `cmd.Wait()`; CA-910 adds the missing evidence rather than guessing.

## Tests

- `TestScaffoldBusy_ShowsSpinnerAndDisablesComposerUntilTerminal`: red because scaffold did not count as live work; then locked spinner, render, keyboard, send, and terminal release behavior.
- `TestScaffoldBusy_ErrorReEnablesComposer`: red because error completion had no busy lifecycle; now confirms the lock releases after an HTTP failure.
- `TestSpawnRunner_RecordsChildExitStatus`: red because runnerboot discarded child exit status; now pins PID/start/exit telemetry.
- Focused TUI init/scaffold tests, runnerboot package tests, and `go build ./...` pass.
- Full `internal/tui/app` still has two unrelated baseline failures: `TestPostDoneFollowUp_StepsPollInSendGapDoesNotSettle` and `TestApprovalBarAndStopAreClickable`. A scoped stash confirmed both fail identically without CA-910.

## Provider Parity

Provider-agnostic. The busy lifecycle wraps the scaffold HTTP request and the process telemetry wraps the runner child; no Claude, Codex, Grok, Devin, or OpenCode adapter behavior changed.

## GitNexus

GitNexus MCP tool discovery failed again, so mandated symbol impact analysis and change detection could not run. Manual impact review found medium TUI risk (`workIsLive`, `handleKey`, `processInput`, `renderInputLine`) bounded by the new lifecycle tests; runnerboot changes only add logging around the existing child wait.

# ---8<--- flowpilot:change-ledger
feature_key: skill-anchored-init
source_doc_id: CP-68
change_type: bugfix
summary: show and enforce scaffold busy state in TUI and record TUI-spawned runner exit status
# --->8---
