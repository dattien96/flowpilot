# BUG-494 — live provider-stream scanners exit on unchecked Err() → output loss + potential child-write deadlock

- Status: `todo`
- Severity: **medium** — when `bufio.Scanner` aborts on an oversized line
  (>8–10 MiB caps) or a pipe read error, the scan goroutine returns while
  the provider child keeps writing → unread pipe can block the child
  (deadlock) and every event after the abort is lost silently.
- Found: 2026-09-25, deep audit pass 2 (companion class to BUG-487 —
  live-stream readers rather than durable-file readers).

## Root cause

`bufio.Scanner` loops over provider process stdout/MCP stdio without
checking `scanner.Err()`:

- `sessions.go:~549` — provider session stdout scanner
- `sessions.go:~864,~882` — Claude print-mode capture goroutine
- `compat.go:~330,~406` — provider stdout parsing
- `firebase_tools_mcp_client.go:164`, `google_drive_proxy_mcp.go:72`,
  `telegram_proxy_mcp.go:61` — MCP stdio scanners

Contrast: `claude_stream.go`, `codex_appserver.go`, `devin_mcp_stdio.go`
already check `Err()` — the unchecked sites are inconsistent outliers.

## Blast radius

1. Provider emits a >cap JSONL frame (large tool result / attachment) →
   scanner aborts mid-stream → all remaining events of that turn lost →
   turn may never produce its terminal frame → looks like a provider hang.
2. If nobody drains the pipe after the goroutine exits, the child blocks on
   write → real deadlock window.
3. Silent: no log distinguishes "stream ended cleanly" from "scan aborted".

## Fix contract

Minimal-risk fix (per `safe-fix-contract`): **check `scanner.Err()` at every
site and log loudly** with provider/session context. Do NOT change parsing
semantics in this pass — the observable contract is "a scan abort is
surfaced", matching how `claude_stream.go` already reports stream errors.

Sites where the Err can be propagated to an existing error channel/result
(e.g. `compat.go` capture paths that already return errors) should
propagate, not just log.

## Required tests (RED first)

- `TestBUG494_StdoutScanAbortIsReported`: feed a pipe/frame stream a >cap
  line → the scan loop's completion path reports/logs the truncation
  instead of clean-EOF silence. Structure the test against the helper
  extraction used for the fix so no real provider process is needed.
- Positive control: clean stream → no error logged.

## Definition of Done

- Every live-stream scanner in the listed files checks `Err()` and surfaces
  it (log at minimum; propagate where an error channel exists).
- Tests green, CA entry, commit.
