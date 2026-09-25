# CA-950 — rebase semantic merge: jsonRpcErrorMessage keeps main append + BUG-381 http_status fallback

## Summary

Rebasing `cp_live_test` onto `main` auto-merged `jsonRpcErrorMessage` into a
franken-function: the BUG-381 early-return (`return data.message`) shadowed
main's BUG-374 append (`msg + ": " + detail`), so `error.message` ("Internal
error") was dropped and `TestBug374JSONRPCErrorKeepsDataDetail` went red.
Reconciled to append-first with the BUG-381 `http_status` fallback retained —
both test files (`bug374_*`, `bug381_*`) green.

## Files

- `apps/local-runner/internal/runner/sessions.go` — `jsonRpcErrorMessage` union

## Verified

- `go test -count=1 -run 'TestBug374JSONRPC|TestBug381_' ./internal/runner/` — ok

## Provider parity

Shared helper used by grok/codex/devin/opencode/gemini dispatchers; covered by
both BUG-374 and BUG-381 test files across the Grok ACP wire shape.
