---
id: CA-474
feature_key: cli-tui
title: TUI or Desktop exit always stops the local runner
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-445-repair-incomplete-tui-bugfixes
will_not_undo: CA-445 Grok session/new cwd+mcpServers; Windows netstat exact port match
```

## Change

CA-445 skipped runner shutdown when TUI reused a listener (`OwnsRunner=false`). `just chat-dev` then attached to a leftover `go run` runner on :4317. `/exit` only closed TUI. Live leak 2026-08-13: PID 18400 spawned 13:40, TUI 14:42 reused it (`run-93161` ran on the stale binary).

Operator rule: reuse a runner if :4317 is already up; when **either** TUI or Desktop exits, stop that runner. Clients are not used at the same time.

- TUI `/exit` / Ctrl-C always `POST /system/shutdown` + port kill.
- TUI spawn no longer detaches (no CREATE_NEW_PROCESS_GROUP / Setsid), so closing the terminal tears down a TUI-spawned runner.
- Runner `/system/shutdown` exits the process after flushing (TUI-spawned runners have no supervisor).
- Desktop Electron `before-quit` POSTs `/system/shutdown`.

## Provider impact

Case 1 (agnostic). Shutdown/spawn process lifecycle; no `providerKey` branch.

## Tests

New files: `internal/tui/app/quit_kills_reused_runner_test.go` (`OwnsRunner=false` and `true` both POST `/system/shutdown`); `internal/tui/runnerboot/sysprocattr_nodetach_test.go` (`SysProcAttr` stays nil). Pre-existing `TestCmdShutdownAndQuit_SkipsKillWhenRunnerReused` only asserts `QuitMsg` and is left untouched. Desktop `before-quit` has no Electron unit harness.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: TUI and Desktop exit always stop the local runner, including a reused :4317 listener
# --->8---
