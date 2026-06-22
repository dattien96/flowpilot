# BUG-115: Provider Accounts Config Re-Saved On Every Resume

## Metadata

- Document ID: `BUG-115`
- Title: `Provider Accounts Config Re-Saved On Every Resume`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: `None`
- Child Documents: `None`
- Related Documents: [BUG-114: Claude spawn_agent Missing When Co-Resident MCP Server Is Slow](./BUG-114-Claude-Spawn-Agent-Missing-When-Co-Resident-MCP-Slow.md)
- Replaces: `None`
- Tags: `provider-accounts, logging, disk-io, runner, observability`

## AI Quick View

### Summary

- During chat navigation the runner log showed `[provider-accounts] config saved` firing repeatedly (2×+ per history-open), each one rewriting `provider-accounts.json`.
- Root cause: `saveProviderAccountState` always wrote the file (and logged) regardless of whether the serialized state differed from what was already on disk. Account load/sync runs on every resume / `ListProviderAccounts`, so the file was rewritten constantly even when nothing changed.
- Fix: make `saveProviderAccountState` a no-op when the serialized payload is byte-identical to the on-disk content — eliminating the redundant writes and the log spam without changing behavior on a real change.
- Bundled observability improvement (requested in the same thread): the runner now tees its log to a file so diagnostics survive past the launching terminal's scrollback.

### Current Ask

- Stop the redundant `provider-accounts.json` writes / log spam during navigation.
- Persist runner logs to a file.

### Key Decisions

- `V-1` Guard the write in `saveProviderAccountState` with a byte-equality check against the existing file. This dedupes ALL callers (list, resume, activate, …) at one chokepoint and never skips a genuine change (the payload differs the instant any field does).
- `V-2` Add `SetupFileLogging` (runner) called from the `serve` command: `log` output is tee'd to stderr AND `<configDir>/FlowPilot/logs/runner.log` via `io.MultiWriter`, with single-generation rotation at 10 MiB. Overridable via `FLOWPILOT_RUNNER_LOG_FILE`. Best-effort: logging setup failure never blocks startup.

### Constraints

- Equality guard compares the exact `json.MarshalIndent` bytes, so formatting is stable across calls.
- File logging captures everything routed through the standard `log` package (`[agent-spawn]`, `[chat-history-open]`, `[claude-mcp]`, `[provider-accounts]`). Lines emitted via `fmt.Println`/`os.Stdout` (e.g. the "listening on" banner) are unaffected.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/provider_accounts.go` — `saveProviderAccountState` equality guard
- `apps/local-runner/internal/runner/logging.go` — `DefaultLogFilePath`, `SetupFileLogging`
- `apps/local-runner/internal/cli/root.go` — `serve` command wires file logging

## 1. Issue Summary

The runner log was dominated by `[provider-accounts] config saved` lines — several per chat-history-open — each rewriting `provider-accounts.json` even when the account state had not materially changed. Separately, runner logs were only available in the launching terminal, making after-the-fact diagnosis hard.

## 2. Parent Links

- related bugfix: [BUG-114](./BUG-114-Claude-Spawn-Agent-Missing-When-Co-Resident-MCP-Slow.md)

## 3. Environment and Reproduction

- environment: Desktop + local runner, any session with provider accounts configured.
- reproduction steps:
  1. Watch the runner log.
  2. Switch between chats in the history panel a few times.
  3. Observe `[provider-accounts] config saved` firing repeatedly with unchanged account counts.
- frequency: Every resume / account list refresh.

## 4. Expected vs Actual

- expected: `provider-accounts.json` is written only when account state actually changes.
- actual: it was rewritten (and logged) on essentially every resume.

## 5. Impact

- users affected: All runner users; cosmetic log noise + unnecessary disk writes during normal navigation.
- severity: Low (no data corruption) but noisy and wasteful; obscured real diagnostics.

## 6. Root Cause

- confirmed cause: `saveProviderAccountState` unconditionally `os.WriteFile`'d and logged. It is invoked from `ListProviderAccounts` and the resume/account-resolution paths, which run on every history-open, so the file was rewritten regardless of whether `syncProviderAccounts` actually changed anything.
- evidence: runner log shows `[provider-accounts] config saved … accounts=7` repeated 2×+ per `[chat-history-open]` with a constant account count.

## 7. Fix Strategy

- `F-1` In `saveProviderAccountState`, read the current file and return early (no write, no log) when `bytes.Equal(existing, payload)`.
- `F-2` Add `SetupFileLogging` + `DefaultLogFilePath`; call from the `serve` command so runner diagnostics tee to `<configDir>/FlowPilot/logs/runner.log` (rotated at 10 MiB; env-overridable).

## 8. Validation

- `V-1` `go build ./...` and `go vet` clean.
- `V-2` Provider-account runner tests pass (33 passed; the 3 failures are pre-existing — two `RestoreChatRunFromDrive…` tests fail identically without this change, and `TestLiveCodexChatResumesAcrossProviderAccountSwitches` requires the `codex` binary).
- `V-3` Manual: with the guard, repeated history-opens no longer emit `[provider-accounts] config saved` unless account state changes.

## 9. Regression Guard

- tests: existing provider-account suite exercises the save path.
- audit checks: a reappearance of constant `config saved` lines during navigation would indicate the guard was bypassed.

## 10. Follow-Up Document Updates

- upstream docs that must change: None.
- notes: file logging is an observability enhancement requested alongside this fix; it does not alter runner behavior.
