# BUG-380: gopls `initialized` notification never sent — post-write diagnostics always empty → LSP pipeline dead in production

## Metadata

- Document ID: `BUG-380`
- Title: `WaitReady returns after initialize without sending initialized → gopls defers package load → only sev2 "No active builds" warnings → AfterFileWriteErrors (sev1 filter) yields nothing`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-63-IDE-Grade-LSP-Runtime](../../07-Coding-Plan/done/CP-63-IDE-Grade-LSP-Runtime.md), [CP-63-Test-Steps](../../07-Coding-Plan/done/CP-63-Test-Steps.md)
- Feature Keys: `lsp-runtime`

## AI Quick View

### Summary

- `internal/lsp/server_manager.go` `WaitReady` (:235-261) calls `client.Initialize(ctx, params)` and returns — it **never sends the LSP `initialized` notification**. `grep '"initialized"' internal/lsp/` finds no production send site (client.go only notifies `exit`/`didOpen`/`didChange`/`didClose`).
- gopls defers workspace/package load until `initialized`; without it, every `didOpen`/`didChange` gets exactly one `publishDiagnostics` carrying severity=2 Warning `No active builds contain <file>` — confirmed for 45s+ by direct probe.
- `PostWriteDiagnosticsHook.AfterFileWriteErrors` (`internal/lsp/runner_hook.go:106-110`) filters `Severity == DiagnosticSeverityError` only → the warning is dropped → `errs` empty → `CheckFiles` always returns `""` → no diagnostics reprompt ever fires, and the `lsp.diagnostics` context source (`GetAllErrors`) is equally empty.
- A/B/C probe proof (`l63-1-probe-variants.py`): A = runner-exact params → warning only; B = +`workspaceFolders` → warning only; C = runner params + **`initialized` notify** → all 5 real sev=1 errors immediately. `initialized` is the sole missing piece.
- Secondary (same doc): `DiagnosticsCollector.WaitForDiagnostics` (`diagnostics_collector.go:100-119`) returns on the first `publishDiagnostics` for **any** URI (generation bump) — a publish for a different file can end the wait before the target file's diagnostics arrive (partial-result race; verify when the primary fix lands).

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.
- Fix direction noted by tester: send `notifications/initialized` (empty params) after a successful `initialize` response inside `WaitReady`/client handshake; consider surfacing the standalone sev-2 "No active builds" warning somewhere (it signals a broken workspace view, not clean code).

## Bug report

- **Symptom**: With gopls on PATH and the LSP feature live (`[lsp] lsp.start` logged, gopls PID child of runner), post-write diagnostics NEVER reach context/events — turns writing files with real compile errors complete clean.
- **Expected** (CP-63 §3.2): after each file write, diagnostics `file.go:line:col error: msg` are injected as a reprompt before tests run.
- **Actual**: cp63 run-131 (opencode) — three gate-clean turns wrote `lsp_probe_oc.go` containing 5 real `undefined:` compile errors; zero `flow_gate_violation{status:"reprompt"}` diagnostics; turns completed clean. `gopls check lsp_probe_oc.go` on the same bed reports all 5 errors. Scratch-copy harness with instrumented collector: `CheckFiles` ×4 → `""` each in ~20–150 ms; every publish = the sev-2 warning.
- **Impact**: HIGH — the entire CP-63 live-diagnostics feature is dead code in production; the post-write hook always returns `""`. Explains the earlier M-1 verification note "hook-injection chưa isolate khỏi agent-tự-chạy-gopls" — the earlier PASS was almost certainly the agent's own `gopls check` output, not the hook.

## Reproduction

1. Runner on :19263 with `gopls` on PATH, bed = Go module (`gatesandbox`).
2. Gate-clean turn writing a file with real compile errors (cp63 turns turn-220/249/278, run-131, provider opencode — each also wrote the CA note + Change Contract so gate `violations=0`).
3. Observe: `[lsp] lsp.start` once (`runner.log:733`); no diagnostics reprompt; turn completes clean.
4. Isolation probe (no runner): send `initialize` with runner-exact params (rootUri + empty capabilities) then `didOpen` → only the sev2 `No active builds` warning for 45s+; repeat with `initialized` notification after `initialize` → 5 real sev1 errors.

## Root cause

- `internal/lsp/server_manager.go:235-261` — `WaitReady` returns after `client.Initialize` response; no `initialized` notification is sent anywhere in `internal/lsp/` (verified by grep — client.go notifies only `exit`, `didOpen`, `didChange`, `didClose`).
- gopls documented behavior: defers workspace/package load until `initialized` → publishes only the placeholder sev-2 warning per file.
- `internal/lsp/runner_hook.go:106-110` — `AfterFileWriteErrors` keeps only `d.Severity == DiagnosticSeverityError` → warning dropped → `""`.
- Secondary: `internal/lsp/diagnostics_collector.go:100-119` — `WaitForDiagnostics` returns on `dc.Generation() != start` (any URI's publish bumps the generation) → partial-result race possible once real diagnostics flow.

## Evidence

- `~/fp-beds/lt-evidence/cp63/RESULT.md` (BUG-LIVE-63-1), `runner.log:733` (`lsp.start`), `l63-1-gopls-proc.txt` (gopls PID 50706, PPID runner 41458), `l63-1-harness-repro.txt` (instrumented CheckFiles → "" ×4, all publishes sev-2 warning), `l63-1-probe-variants.py` (A/B/C isolation), `l63-1-run131-events.json`.
- Degrade path verified healthy (L-63-2 PASS) and crash budget works (L-63-3 PASS: `lsp.restart` → `lsp.disabled (crash budget spent)`) — the defect is purely the missing `initialized` handshake.

## Severity

- high

## Completion Notes (implemented 2026-09-23, CA-918b)

- Primary: `Client.Initialized()` sends the LSP `initialized` notification;
  `ServerManager.WaitReady` calls it after a successful `initialize` — gopls
  now loads the workspace and publishes real sev-1 diagnostics.
- Secondary: `DiagnosticsCollector` tracks per-URI publish generations;
  `WaitForDiagnosticsForURI` (used by `AfterFileWriteErrors`) returns only on
  the target URI's publish — foreign-file publishes no longer end the wait.
- Files: `internal/lsp/{client,server_manager,diagnostics_collector,runner_hook}.go`.
- Tests: `bug380_initialized_handshake_test.go` (3 tests; helper process
  records notification methods). Full `internal/lsp` green.
