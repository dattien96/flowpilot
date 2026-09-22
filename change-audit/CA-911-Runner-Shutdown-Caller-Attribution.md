---
id: CA-911
title: Runner shutdown/restart caller attribution logging
type: BugFix
feature: skill-anchored-init
date: 2026-09-22
status: done
---

## Context

The runner process vanished mid-scaffold twice (`POST /client/projects/{id}/scaffold`
died with `wsarecv: An existing connection was forcibly closed`). CA-910 added
`[runnerboot] runner pid=N exited: <err>` telemetry, which proved the runner exits
**cleanly** (`<nil>` = `os.Exit(0)`).

Forensics on the 11:29 incident:

- `.flowpilot/supervisor.cmd` was rewritten with content `shutdown` at the exact
  crash minute — that file is only written by the `POST /system/shutdown` handler.
  So the handler definitely ran; an HTTP POST arrived.
- Every known caller was eliminated at the time: the TUI stayed alive with no
  quit event and zero key input (input-watchdog stall covers the whole window),
  no desktop Electron, no admin-web, no `just dev` supervisor (the TUI was
  launched via `just chat-dev`, which does not use `scripts/supervisor.js`), and
  the scaffold's Devin agent only executed read-only `ls`/`find`/`cat` commands.
- The handler logged nothing about the caller, so the requester is unidentified.

## Change

`internal/cli/root.go`: both `/system/shutdown` and `/system/restart` handlers now
log the requester before tearing down:

```text
[runner] /system/shutdown requested remote=127.0.0.1:NNNNN ua="Go-http-client/1.1"
```

`remote` + `ua` distinguish the possible callers: Go client (TUI / CLI helpers),
undici/node fetch (desktop app, admin-web, supervisor.js), curl/scripts, or a
provider-spawned agent shell. The next reproduction identifies the killer.

## Test

- `internal/cli/shutdown_requester_telemetry_test.go` (new, additive): asserts
  both handlers log `r.RemoteAddr` and `r.UserAgent()` — source assertion, same
  pattern as `runner_exit_telemetry_test.go`.

## Provider parity

Provider-agnostic: HTTP handler logging only; no adapter/event-stream/session
code touched.

## Risk

Low — two `log.Printf` additions, no behavioral change. `cmd.Wait()` result and
the supervisor handshake are unchanged.

## Blast radius (manual; GitNexus MCP unreachable)

Anonymous mux closures in `runner serve` RunE — no callers, no symbol signature
changes. Runtime effect: two extra log lines per shutdown/restart request.
