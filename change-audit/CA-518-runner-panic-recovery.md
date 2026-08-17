---
id: CA-518
feature_key: cli-tui
title: Runner mux panic-recovery middleware; panic no longer kills the process
date: 2026-08-15
status: COMPLETE
---

## Problem

The runner served HTTP through a bare `http.ListenAndServe(addr, withCORS(mux))`
with no panic-recovery around the mux (cli/root.go). Any panic on a request
goroutine — e.g. inside a child `resumeRun` or a Grok JSONL transcript seed —
propagated out of the handler, the net/http server logged it and the goroutine
died, but a panic that escapes also crashes the whole process in Go's runtime
view when the default net/http server is used with the default behavior? In
practice the server's `ServeHTTP` recovery only catches its own internal
panics; a handler panic escapes `ServeHTTP` (net/http does NOT recover handler
panics by default), so the process crashes.

Result: one panic in a child `resume` / `POST /resume` / Grok seed killed the
entire runner. Every later client call (including the TUI `[open]` retry after
CA-517) then hit `dial tcp … connection refused` even though the runner had
been healthy moments earlier — the recurring `connection refused` symptom.
The runner process died; the TUI kept showing cached main transcript, so the
runner looked alive while being gone.

## Fix

Added `withRecovery` middleware (cli/root.go) wrapping the mux outermost:

```
http.ListenAndServe(addr, withRecovery(withCORS(mux)))
```

- `defer recover()` converts any handler panic into a 500 JSON error
  (`{"error": "internal panic on <method> <path>: <value>"}`) using the
  existing `writeHTTPError` contract, so the TUI client's `parseAPIError`
  parses it as a normal server-side error (non-dial → CA-517 falls back to the
  live stream instead of hard-failing).
- Logs the panic value + `runtime/debug.Stack()` via the existing `log`
  package so the root cause is visible in the runner output.
- The process stays alive; only the offending request gets a 500.

## Provider impact

Agnostic — wraps every runner route regardless of provider. Panics in Claude /
Codex / Grok resume/seed handlers all become 500s instead of process death.

## Tests

`internal/cli/runner_recovery_test.go` (additive, new file):

- Handler panic → 500 JSON, and the process (httptest server) survives a
  subsequent request.
- Healthy request passes through `withCORS(withRecovery(next))` unchanged:
  status, body, and CORS headers intact (no regression to CORS path).

## Notes

- Only protects panics on the request goroutine. Panics inside separately
  spawned goroutines (e.g. an event pump started without recover) still crash
  the process — those are debugged from the `PANIC` log line this middleware
  now surfaces for the request path.
- Pairs with CA-517: TUI no longer hard-fails on resume error, and the runner
  now survives the panics that caused the recurring connection refused.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Wrap the runner mux in panic-recovery middleware so a handler panic returns a 500 JSON error and logs a stack trace instead of crashing the process and turning every later client call into connection refused
# --->8---