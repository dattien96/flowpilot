# CA-993 — BUG-494: live stream scanners surface scanner.Err()

## What changed

`apps/local-runner/internal/runner/`:

- `sessions.go`: new `scanStreamLines(scanner, fn)` helper — copies each
  line stably and returns `scanner.Err()`. The Claude print-mode stdout and
  stderr capture goroutines now route through it; captured scan errors are
  joined and returned as `provider error: Claude stream read failed: ...`
  after `cmd.Wait()` — a truncated capture can no longer masquerade as a
  clean exit with a misleading "no result event" complaint.
- `compat.go` claude stream-json probe: post-loop `scanner.Err()` → fail
  with "stdout read error" instead of a false "missing frame type".
- `compat.go` codex initialize probe: scan termination sends `Err()` on a
  dedicated channel — the select distinguishes "read fault" (fail) and
  "stream closed without response" (warn) from the timeout warn.

## Provider parity

Audited every live-stream reader in the package. Claude stream-json
(`claude_stream.go`), Grok (`grok_process.go`), Devin (`devin_process.go`),
Opencode (`opencode_process.go`), Codex app-server (`codex_appserver.go`),
MCP stdio servers, and `readJsonRpcMessage` all already surface
`scanner.Err()` — the capture goroutines and the two compat probes were
the only unchecked sites.

## Verification

- `bug494_stream_scanner_err_test.go`: mid-stream read fault surfaces the
  error with prior lines still delivered; >10 MB line surfaces
  `bufio.ErrTooLong` (was silent EOF); clean EOF and empty readers return
  nil.
- Runner package suite green.
