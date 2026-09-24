# CA-950 — KR-005 F-1: runnerboot_cp81_test.go compile failure on Windows

## Summary

`runnerboot_cp81_test.go` asserted `cmd.SysProcAttr.Setsid` inline — that field
only exists in the Unix `syscall.SysProcAttr`, so the whole runnerboot test
package failed to compile on Windows (`runtime.GOOS != "windows"` runtime
guard cannot save a compile-time field access). All 13 CP-81 runnerboot tests
(reuse/replace/fenced-kill/single-winner) were silently un-runnable on
Windows.

Fix: platform-split helper `assertSharedRunnerDetached(t, cmd)` — Unix variant
checks `Setsid`, Windows variant checks the detached-process creation flags.
Net coverage *increased*: Windows previously skipped the assertion entirely.

## Files

- `apps/local-runner/internal/tui/runnerboot/runnerboot_cp81_test.go` — inline
  `Setsid` assert → `assertSharedRunnerDetached(t, cmd)`
- `apps/local-runner/internal/tui/runnerboot/sysprocattr_shared_unix_test.go` — NEW
- `apps/local-runner/internal/tui/runnerboot/sysprocattr_shared_windows_test.go` — NEW

## Verified

- `go test ./internal/tui/runnerboot` on Windows: 7.6s, all pass.

## Provider parity

Provider-agnostic: process-spawn mechanics only.
