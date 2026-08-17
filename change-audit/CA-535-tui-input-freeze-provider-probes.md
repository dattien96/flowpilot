---
id: CA-535
feature_key: cli-tui
title: Fix intermittent TUI input freeze on startup (Windows provider probes)
date: 2026-08-17
status: COMPLETE
---

## Problem

On Windows, occasionally right after the TUI starts the operator cannot type
into the chat box for ~10 seconds: the chat box is visible, keys are dropped
(not queued), and the cursor is frozen until the runner finishes a provider
scan. Not the "chat disabled (please wait)" overlay — the input itself stalls.

Root cause chain (verified in code):

1. `cmdConnect` → `ConnectedMsg` sets `sessionLoading=true` and schedules
   `cmdLoadSessionDefaults`.
2. `cmdLoadSessionDefaults` calls `ListProviderAccounts` + `ListProviders`
   (`GET /providers`) under a shared 8s `ctxFast`.
3. The `/providers` handler ran `instance.DetectProviders(context.Background())`
   — **server-side the request context was ignored**.
4. `DetectProviders` spawns 4 CLI probes (`codex`, `claude`, `agy`/`gemini`,
   `grok`) via `resolveVersion` (`--version`) and `runCommandFn` (`codex debug
   models`, `gemini models`). The runner process is intentionally not detached
   (CA-474), so these probes inherit the TUI's console; on Windows a probe that
   attaches to the parent console steals/swallows console input for its
   lifetime → TUI cursor freezes ~8–12s.
5. The TUI client's 8s `ctxFast` timeout did not save us because the server
   used `context.Background()`.

## Fix (runner + TUI, additive)

1. **`/providers` honors the request context** — `cli/root.go` now calls
   `instance.DetectProvidersCached(r.Context())`. When the TUI's fast-path
   context is cancelled, the server aborts the scan and every in-flight probe
   is killed via `exec.CommandContext`.
2. **Probes never attach to the console** — new `runner/probe_cmd.go`
   (`newProbeCmd`): stdin is always wired to `os.DevNull`, and on Windows
   (`probe_cmd_windows.go`) `CREATE_NO_WINDOW` (0x08000000) is set so the
   probe never allocates or inherits a console window. `resolveVersion` and the
   default `runCommandFn` now build commands through `newProbeCmd`. Probes that
   can't steal console input can't freeze the TUI even while they run.
3. **Short-TTL `DetectProviders` cache** — `runner.go` adds
   `DetectProvidersCached` (3s TTL, mutex-guarded) used by the `/providers`
   handler, so a quick TUI restart does not re-scan all 4 CLIs. The pure
   `DetectProviders`/`ListProviders` stay uncached (InstallProvider re-lists
   after install and must observe fresh state). Every mutation that changes the
   inventory invalidates the cache: `InstallProvider`, `AuthenticateProvider`,
   `ConnectProviderAccount`, `VerifyProviderAccount`,
   `ActivateProviderAccount`.
4. **Providers decoupled from the critical unlock path** — `app.go`
   `cmdLoadSessionDefaults` now gives `ListProviders` a short **2s independent
   budget** instead of sharing the 8s accounts window. A cold probe scan can no
   longer hold the session unlock for the full fast-path window; combined with
   1–3 it resolves in milliseconds in the common case. The catalog path is
   unchanged (still parallel, CA-514 contract preserved). A providers ctx
   deadline is treated like a catalog deadline — never reported as
   "runner offline" (CA-514 rule).

## Provider impact

Provider-agnostic (Case 1): `newProbeCmd`/`applyProbeSysProcAttr` apply the
same attributes to every provider probe; the cache is per-runner, not
per-provider; the TUI change is provider-neutral. Claude/Codex/Gemini/Grok
detection all route through `resolveVersion`/`runCommandFn`.

## Tests

New additive files:

`runner/ca535_provider_cache_test.go`:

- `TestDetectProvidersCached_ReusesResultWithinTTL` — warm cache does not
  re-probe (probe counted via wrapped `lookPathFn`).
- `TestDetectProvidersCached_ExpiresAfterTTL` — expired cache re-probes.
- `TestDetectProvidersCached_InvalidationForcesReprobe` — mutation invalidation
  forces a fresh scan.
- `TestProbeCmd_NeverAttachesConsole` — probe stdin is always DevNull-backed.
- `TestDetectProvidersCached_NilRunnerIsSafe` — nil receiver guard.
- `TestDetectProvidersCached_ErrorPassthrough` — errors surface despite cache.

`tui/app/ca535_providers_budget_test.go`:

- `TestCmdLoadSessionDefaults_ProvidersScanDoesNotHoldUnlock` — a `/providers`
  mock that sleeps 3s (> 2s budget) must not delay the session unlock past the
  budget + buffer, and its timeout must not be reported as runner-dead.

## Verification

- `go build ./...` clean; `go vet ./...` clean except pre-existing
  `internal/structure/gitnexus.go:272` (untouched).
- gofmt clean on LF-normalized copies (this checkout is CRLF repo-wide; new
  files written CRLF).
- `go test ./internal/tui/app/... -count=1` green (skipping the two known
  pre-existing flaky `TestCmdFocusAgent_*` network tests).
- `go test ./internal/runner/... -count=1` green except pre-existing flaky
  `TestSyncChatRunToDriveConcurrentIndexMergesPreserveAllParentRows`
  (fails ~2/3 on the clean baseline too — confirmed with changes stashed) and a
  one-off `TestGrokGoldenFixtureStreamedPromptAndToolCall` flake that passes in
  isolation and on rerun.
- Legacy CA-514 contract tests untouched and green: `tui_no_freeze_catalog_test.go`,
  `tui_runner_offline_nofreeze_test.go`, `chat_ux_test.go`.

## Out of scope / residual

- Runner spawn detachment (CA-474) is intentionally preserved — the runner must
  die with the TUI terminal; the console-steal is fixed at the probe level
  instead.
- Only `resolveVersion` and the default `runCommandFn` were converted to
  `newProbeCmd`; other one-off exec sites (compat diagnostics, firebase MCP)
  are not on the TUI startup path.
- Cache TTL fixed at 3s; not configurable.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CA-535
change_type: bugfix
summary: Fix intermittent TUI input freeze on Windows by cancelling provider probes with the request context, detaching their console (CREATE_NO_WINDOW + DevNull stdin), caching detection for 3s, and giving the providers fetch a short independent budget
# --->8---