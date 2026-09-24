# CA-966 — Devin ACP prewarm (boot/login/switch) + provider_status cold-start event (Task-439)

## Summary

The first Devin chat turn paid the `devin acp` cold start — spawn +
`initialize` + PKCE `authenticate` (browser) — synchronously inside adapter
resolution, which reads as a hang for tens of seconds. Later turns reuse the
cached process, so only the first turn suffered.

1. **Prewarm triggers** (`devin_prewarm.go`):
   - **Boot**: `WarmDevinProcessAsync("boot")` after `AttachRunner` in
     `newRunnerCommand` — warms whenever a *connected* devin account exists,
     regardless of the currently-selected provider. Fails closed for
     unauthenticated installs (never pops a browser unprompted).
   - **Login**: `VerifyProviderAccount` warms the just-verified account
     (which may not be the active slot yet) via `warmDevinAccountAsync`.
   - **Account switch**: `ActivateProviderAccount` warms the newly-active
     account; the foreign-base reclaim inside `reuseDevinProcessLocked`
     closes the previous account's process (account isolation kept).
   - Dedup via `devinPrewarmInFlight` keyed by account scope — boot+login+
     switch landing together pay one spawn. The warm handle lands in the
     chat segment ("") so a turn's `ensureDevinProcess` reuses it; sessions
     are still minted per turn inside the shared process — chat/session
     structure untouched.
   - Spawn ctx is `context.Background()` — `exec.CommandContext` ties the
     process's life to ctx, so a bounded warm ctx would kill the just-warmed
     process on cancel; init/auth stay bounded by `devinInitTimeout` /
     `devinAuthTimeout` inside `ensureDevinProcessSegmented`.
   - Under `go test`, warming requires `FLOWPILOT_DEVIN_BIN` opt-in (same
     ambient-binary guard as `warmDevinModelsCacheAsync`) — no real `devin`
     binary ever spawns in tests.

2. **`provider_status` event** (`EventProviderStatus` = `"provider_status"`):
   `startTurn` predicts a Devin cold start via `devinChatProcessWarm()` (no
   live chat-scope handle for the resolved account) and emits
   `connecting` before `AdapterWithScope`, then `ready`/`failed` after —
   subscribers see progress during the silent PKCE window instead of a
   frozen UI. Emitted under `s.mu` before the blocking factory call, so it
   reaches subscribers while the handshake runs.

3. **Desktop + TUI**: `ProviderEventDTO` gains the `provider_status` variant;
   `timelineReducer` renders one system row per run
   (`provider-status-<runId>`) — `connecting` pushes an info row,
   `ready`/`failed` resolve it in place (failed → error tone). No
   `Thinking...` row is spawned for it. The TUI surfaces it on the
   statusline (`statusMsg = ev.Text`) and as an error transcript line on
   `failed`.

## Changes

- `apps/local-runner/internal/runner/devin_prewarm.go` — new.
- `apps/local-runner/internal/runner/devin_prewarm_test.go` — new (6 tests:
  boot warm, no-account guard, login trigger, switch warm+reclaim,
  `devinChatProcessWarm` predicate, startTurn emits connecting-while-blocked
  then ready).
- `apps/local-runner/internal/runner/provider_accounts.go` — hooks in
  `VerifyProviderAccount` / `ActivateProviderAccount` (devin-gated).
- `apps/local-runner/internal/runner/provider_event.go` —
  `EventProviderStatus` const.
- `apps/local-runner/internal/runner/interactive_service.go` — cold-start
  emit around `AdapterWithScope`.
- `apps/local-runner/internal/runner/devin_adapter.go` — `devinLaunchEnv`
  refactored onto shared `devinAccountEnv` (no behavior change).
- `apps/local-runner/internal/cli/root.go` — boot warm call.
- `apps/desktop-flowpilot/src/types/contract.ts` — DTO variant.
- `apps/desktop-flowpilot/src/state/timelineReducer.ts` — `provider_status`
  case (in-place row resolution).
- `apps/desktop-flowpilot/src/state/timelineReducer.test.ts` — 4 additive
  tests appended.
- `apps/local-runner/internal/tui/app/app.go` — `provider_status` case
  (statusline + error line on failure).

## Tests

- `go test ./internal/runner -run 'WarmDevin|Prewarm|DevinChatProcessWarm|
  StartTurnEmitsDevinColdStart'` — 6/6 green (red-first by assertion).
- `timelineReducer.test.js` — 36/36 green (4 new).
- `tsc --noEmit` — clean.
- Provider parity: `provider_status` is provider-neutral in contract but
  emitted only on the devin path (guard `providerKey == ProviderKeyDevin`);
  Claude/Codex/Grok/Gemini/Opencode turn flow unchanged — no adapter, session,
  or event-shape change reaches them. The reducer renders the row generically
  for any provider that emits it later.
- Known flake (pre-existing, unrelated): `TestStartTurnGrokCrossAccount…`
  intermittently fails TempDir cleanup on Windows (`directory not empty`);
  passes standalone.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: Task-439
change_type: feature
# --->8---
