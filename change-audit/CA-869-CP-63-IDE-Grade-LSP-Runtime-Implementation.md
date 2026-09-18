# CA-869 — CP-63 IDE-Grade LSP Runtime implementation (P-1 → P-8)

# ---8<--- flowpilot:change-ledger
feature_key: lsp-runtime
source_doc_id: CP-63
change_type: feature
summary: implement CP-63 P-1 through P-8 (Tasks 354-361) — embedded LSP client over stdio, server lifecycle manager, platform registry, document sync and diagnostics collector, post-write diagnostics hook in the gate allow-path, CP-44 lsp.diagnostics context source, Kotlin init options, Gradle fallback validator
# --->8---

## Why

FlowPilot relied on GitNexus (static graph, stale after every agent edit) plus full builds (5–120s) for compiler feedback. CP-63 adds the Micro Plane: live diagnostics from real language servers in <200ms, auto-reprompting the agent before any test command runs.

## Change

- `apps/local-runner/internal/lsp/` (new package, zero non-stdlib deps):
  - P-1 `protocol.go` (LSP 3.17 subset) + `client.go` (JSON-RPC over stdio, background demux, per-request channels, 5s default timeout, malformed-frame resync).
  - P-2 `server_manager.go` (spawn/stop/restart, health loop, max 1 crash-restart per session then disable, `WaitReady` handshake, stderr drain; additive `Env` + `InitOptions` fields).
  - P-3 `platform_detect.go` (`DetectPlatform`, shared with the TUI wizard) + `platform_registry.go` (7 platforms, `DetectAndResolve` via PATH, `LanguageID`, per-platform `ConfigForFile`).
  - P-4 `doc_sync.go` (version-tracked open/change/close) + `diagnostics_collector.go` (per-URI store, severity filter, generation wait, `file:line:col` formatter).
  - P-5 `runner_hook.go` (`PostWriteDiagnosticsHook` + multi-server `ServerSet` keyed by workspace+language with lazy-start, idle reap, failure cooldown).
  - P-6 `context_source.go` (`lsp.diagnostics` source logic + `ServerSet` error aggregation).
  - P-7 `kotlin_config.go` (SDK detection, Gradle settings, compose heuristic, per-workspace server config).
  - P-8 `gradle_fallback.go` (Android-only trigger, `compileDebugKotlin` runner, output parser, formatter).
- `apps/local-runner/internal/runner/`:
  - `lsp_hook.go` (new): `lspChecker` interface, `lspDiagnosticsForTurn`, `blockTurnForLSPDiagnostics` reusing the existing reprompt fields/settle chain; `lspChecker` field on `InteractiveService` (nil = pre-CP-63 behavior).
  - `gate_hook.go`: 4-line insertions in both gate allow-paths (contract/scope first, compiler second; nil checker returns immediately).
  - `context_source_lsp.go` (new) + registration in `registerBuiltinContextSources` (opt-in only, NOT in `defaultContextSourceIDs`; priority 5).
- `apps/local-runner/internal/tui/app/project_wizard.go`: `detectProjectPlatform` delegates to `lsp.DetectPlatform` (behavior identical, old tests untouched).

## Deliberate deviations from the drafts (all test-covered)

- P-3: `ConfigForFile(platform, path)` instead of path-only (extensions are ambiguous across platforms); python binary is `pyright-langserver` (the real LSP server, not the `pyright` CLI); added `LanguageID()` helper.
- P-5 (post-discovery): query at turn end via `fin.ChangedFiles` (never block inside `emitLocked`, which holds `s.mu`); deliver via existing reprompt machinery (no new turn management); no mid-turn delivery (single-turn model).
- P-6: adapter split (`lsp.Source` + runner adapter) forced by the import graph (lsp must not import runner).
- P-7: thin handshake wiring (`InitOptions` field + `InitOptionsFor` hook) so options reach the server instead of staying dead config.
- P-8: library only, no runner call-site (deferred follow-up, as Task-361 records).

## Tests

- `internal/lsp`: 66 tests green (58 per CP-63 signatures + 8 additive: LanguageID, ServerSet routing/skip/missing, aggregation, handshake capture, hook shape). Cross-platform fakes throughout (.bat/sh, helper-process mock servers).
- `internal/runner`: new `lsp_hook_test.go` + `context_source_lsp_test.go` green; all pre-existing context-source/registry/catalog tests green; gate regression batch re-run — only pre-existing HEAD failures remain (verified via stash), plus one load-flake that passed on re-run and in isolation.
- `internal/tui/app`: full suite — only the 2 pre-existing HEAD failures; all wizard/onboarding tests green (P-3 refactor parity held).
- Pre-existing failures NOT introduced here (verified failing at HEAD with changes stashed): `TestApprovalBarAndStopAreClickable`, `TestPostDoneFollowUp_StepsPollInSendGapDoesNotSettle`, `TestSupabaseCatalogStoreShaping`, `TestCatalogStoreForFallsBackToFake`, `TestFinalizerHookSurfacesArtifacts`, `TestRun144900_PreexistingSkillDoesNotCauseScopeDrift`, `TestTask335_DriftStateMap_BoundedEviction`, `TestRootFlowEngineDefersCompletedUntilGate`; full runner suite also hangs environmentally in `grok_process_test`.

## Providers

Provider-agnostic. Evidence: the hook lives in the shared post-turn gate path (`runFlowGateAtEpoch`/`runChildArtifactOutputGateAtEpoch`) used identically by Claude/Codex/Grok/OpenCode adapters; diagnostics flow through provider-neutral `EventFileChanged`/`finalizeInput.ChangedFiles`; no adapter or provider-selection code touched (only mappers already emitting the event were read, never modified).

## Prior CA claims kept intact

- CA-867/CA-868 (cli-tui): wizard detection behavior byte-identical via delegation; modal lifecycle, gate evaluation (`Evaluate`/`Enforce`), reprompt settle chain, and default context-source set all unmodified.
- CP-63 review protocol: each slice has its Task doc (354–361, now `done`) and this CA entry; GitNexus impact run before touching `detectProjectPlatform` (LOW), both gate functions (LOW), and `registerBuiltinContextSources` (LOW); re-analyzed the index when stale.

## Follow-ups (recorded in task docs)

- C/C++ (clangd) needs platform detection markers + registry entry (Task-356).
- Gradle-fallback call-site wiring into the hook path (Task-361).
- `LSPNavigationSource` deferred (Task-359); Q-1 JetBrains Kotlin LSP watch (Task-360).
