---
id: CA-918b
title: LSP — send `initialized` handshake + URI-scoped diagnostics wait (BUG-380)
type: BugFix
feature: lsp-runtime
date: 2026-09-23
status: done
---

## Context

cp63 live evidence (BUG-LIVE-63-1): the entire CP-63 post-write diagnostics
feature was dead code — `WaitReady` returned after `initialize` without ever
sending the LSP `initialized` notification. gopls defers workspace/package
load until it arrives, so every didOpen/didChange yielded only the sev-2
"No active builds" placeholder, which the sev-1-only `AfterFileWriteErrors`
filter drops → `CheckFiles` always returned "".

A/B/C isolation probe proved `initialized` is the sole missing piece: runner
params + `initialized` notify → all 5 real sev-1 errors immediately.

## Change

- `internal/lsp/client.go` — `Client.Initialized()`: sends the
  `initialized` notification (empty params) per the LSP lifecycle.
- `internal/lsp/server_manager.go` — `WaitReady` sends `initialized` after a
  successful `initialize` response; failure wraps as
  `lsp: wait ready (initialized)`.
- `internal/lsp/diagnostics_collector.go` — per-URI publish counter
  `byURIGen` + `WaitForDiagnosticsForURI(ctx, uri, timeout)` (secondary
  defect in the same report): the generation-wide wait returned on ANY
  file's publish, so a foreign file's diagnostics could end the wait before
  the target file's own publish arrived.
- `internal/lsp/runner_hook.go` — `AfterFileWriteErrors` now waits on the
  target URI's publish, not any publish.
- `internal/lsp/server_manager_test.go` — helper mode `lspserver` now records
  every incoming notification method to `GO_LSP_HELPER_METHODS` (additive
  test-infra seam).

## Tests

- `bug380_initialized_handshake_test.go`
  - `TestBug380_WaitReadySendsInitializedNotification` — helper LSP process
    asserts the `initialized` notification arrives after initialize
    (reproduce-first: red before the fix, waited the full deadline).
  - `TestBug380_WaitForDiagnosticsForURIIgnoresOtherFiles` — foreign URI
    publish must not end the target wait.
  - `TestBug380_WaitForDiagnosticsForURIRequiresFreshPublish` — a stale
    pre-call publish does not satisfy the wait.
- Full `internal/lsp` package green (48.4s); runner LSP hook tests green.

## Provider parity

Provider-agnostic — LSP path is below the provider seam; the fix benefits
every provider's post-write diagnostics equally (the bug was proven on
opencode but reproduces identically for any adapter writing Go files).

## Live evidence

- `~/fp-beds/lt-evidence/cp63/RESULT.md` (BUG-LIVE-63-1),
  `l63-1-probe-variants.py` (A/B/C), `l63-1-harness-repro.txt`,
  `runner.log:733`, `l63-1-run131-events.json`.
- Degrade/crash-budget paths were already verified live (L-63-2/3 PASS) —
  only the handshake was missing.

# ---8<--- flowpilot:change-ledger
feature_key: lsp-runtime
source_doc_id: BUG-380
change_type: bugfix
summary: LSP initialized handshake plus URI-scoped diagnostics wait
# --->8---
