# CA-870 — LSP missing-server warning (sidebar + API)

# ---8<--- flowpilot:change-ledger
feature_key: lsp-runtime
source_doc_id: Task-362
change_type: feature
summary: add missing language-server warning UX — per-platform install hints, pure-query ServerSet.Status, warn-once gate logging, GET /client/lsp-status endpoint, TUI right-sidebar 2-line warning, desktop client contract method
# --->8---

## Why

Post-CP-63 review found two gaps when the user's machine lacks the language server binary: (1) the gate logged `[lsp] check skipped` on every code-touching turn (spam); (2) the user had no visible signal what was missing or how to install it.

## Change

- `internal/lsp/platform_registry.go`: `InstallHint` for all 7 platforms.
- `internal/lsp/lsp_status.go` (new): `ServerStatus` + pure-query `ServerSet.Status()` (no spawn, safe to poll).
- `internal/lsp/runner_hook.go`: `warned` map (missing binary warns once per session); removed per-turn skip logging; Start/WaitReady failures log at attempt time (cooldown-gated).
- `internal/runner/lsp_status.go` (new) + `GET /client/lsp-status?path=` route (1 line in `interactive_handlers.go`).
- `internal/tui/client/lsp_status.go` (new): `LSPStatus` + `GetLSPStatus`.
- TUI app: `lspStatus`/`lspStatusPath` fields, `LSPStatusMsg` with stale-drop, `cmdFetchLSPStatus` triggered on SessionDefaults firstLoad + ProjectCreated, `lspSidebarLines` (max 2 width-fitted rows after the session header; steps budget auto-shrinks, no viewport overflow).
- Desktop: `LSPStatus` type + optional `getLSPStatus(cwd)` in contract + `HttpWsRunnerClient` implementation. Electron sidebar rendering is an explicit follow-up (not verifiable here).

## Tests

- 5 new lsp tests (installed/missing/unknown status, purity, warn-once via captured log, hints present).
- 3 new runner handler tests (400/200-missing/200-installed over httptest).
- 2 new TUI client tests (parse + error).
- 6 new TUI app tests (render present/absent/installed/truncated, stale-drop, fetch trigger).
- Pre-existing suites: lsp full green; tui/client full green; tui/app full green except the 2 pre-existing HEAD failures (`TestApprovalBarAndStopAreClickable`, `TestPostDoneFollowUp_StepsPollInSendGapDoesNotSettle`, both verified failing without these changes); no pre-existing test edited.
- Desktop `tsc --noEmit` not runnable (node_modules not installed); TS changes mirror adjacent patterns exactly.

## Providers

Provider-agnostic. Evidence: status computation touches only platform detection + PATH lookup; endpoint is provider-neutral; TUI rendering is provider-independent chrome. No adapter or provider-selection code touched.

## Prior CA claims kept intact

- CA-869 (CP-63): gate allow-path behavior unchanged when servers exist (warn-once only alters the missing-binary log path); `CheckFiles` contract ("" on degradation) unchanged; no default-source, modal, or gate-logic changes.
