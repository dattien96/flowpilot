---
id: CA-913
title: Resolve /system/shutdown caller to a concrete PID at request time
type: BugFix
feature: runner-observability
date: 2026-09-22
status: done
---

## Context

CA-911's requester telemetry caught the mystery runner kill live:

```text
[runner] /system/shutdown requested remote=127.0.0.1:59519 ua="Go-http-client/1.1"
[runnerboot] runner pid=4804 exited: <nil>
```

That proved a POST — not signal, panic, or timeout — but `ua="Go-http-client/1.1"`
is the default for every Go binary on the box (TUI, `flowpilot.exe` MCP
children, any Go tool). Post-hoc netstat shows nothing because the client
socket is already gone. Ruled out by evidence: TUI stayed alive with no
QuitMsg, devin agent ran only read-only `ls`/`find`, desktop callers are
undici not Go, and no MCP tool or CLI subcommand reaches the endpoint.

## Change

At request time — while the client socket is still open — resolve the remote
port back to the owning local process and log it before tearing down:

- `internal/cli/shutdown_requester.go` (new): `describeRequester(r)` renders
  `remote=… ua=… xclient=… proc=pid=N name=X` for the system-control log line.
  `remoteOwnerPID` runs `netstat -ano -p tcp` on Windows (client row = the one
  whose LOCAL endpoint equals `r.RemoteAddr`) or `lsof -Fp` on unix, both
  bounded by a 4s context; `processName` maps PID → exe via `tasklist`/`ps`,
  3s bound. All best-effort: failure degrades to the CA-911 fields, never
  blocks or delays teardown.
- `root.go`: `/system/shutdown` and `/system/restart` now log
  `describeRequester(r)` instead of the bare remote+ua pair.
- `tui/client/client.go` `ShutdownStack`: sets `X-Client: tui` so future
  TUI-initiated quits are distinguishable from foreign Go clients sharing the
  same default UA.

Next reproduction will name the killer exactly, e.g.
`proc=pid=17264 name=flowpilot.exe` vs `proc=pid=NNNN name=other.exe`.

## Tests

- `TestParseOwnerPID_NetstatWindows` / `_IgnoresServerRowAndMissing` /
  `_LsofUnix`: pure parser coverage for the netstat/lsof row shapes.
- `TestDescribeRequester_IncludesClientMarkerAndProc`: injectable lookup seam
  proves the log line carries remote+ua+xclient+proc.
- `TestSystemShutdown_LogsRequesterIdentity` (CA-911): tightened — handlers
  must log `describeRequester(r)`, which strictly supersedes the old
  remote+ua assertion.
- `X-Client` marker verified by the describeRequester test path.

## Risk

- Observability-only; no shutdown/restart semantics changed. Lookup is
  bounded (4s+3s contexts) and runs once per request on a rare endpoint.
- `lookupRequesterProcess` is a package-level var for injection — same
  pattern as other runner test seams.
- GitNexus unavailable during this change; impact assessed manually (handler
  body + new file, no signature changes).
