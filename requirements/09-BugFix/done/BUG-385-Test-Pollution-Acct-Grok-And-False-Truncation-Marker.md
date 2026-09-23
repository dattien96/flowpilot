# BUG-385: Test-pollution hygiene — synthetic `acct-grok` persisted in machine-global provider-accounts.json + false truncation marker on assistant-less turns

## Metadata

- Document ID: `BUG-385`
- Title: `Go-test synthetic account leaks into real provider-accounts store; handoff renderer appends "[turn truncated]" marker to turns that were never truncated`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-46-Grok-Build-Controlled-Adapter-Over-ACP](../../07-Coding-Plan/done/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md)
- Feature Keys: `ai-providers, cross-provider-handoff`

## AI Quick View

### Summary

- Two small hygiene defects captured together (split later if triage prefers):
  1. **Store pollution**: a synthetic test account `acct-grok` (id format only produced by test helpers; `home_path` into a `go test` TempDir that no longer exists; `auth_status:failed`, slot 1) is persisted in the machine-global `~/Library/Application Support/FlowPilot/provider-accounts.json` and is returned by `/provider-accounts` and `/client/provider-accounts` to real consumers. Leaked by an earlier `go test` run on the machine (created 2026-09-21T14:59:27Z, ~7.5 h before the cp46 session's suite) — a test wrote through to the real user config dir instead of an isolated `FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH`/HOME.
  2. **False truncation marker**: `renderConversationTurn` (`apps/local-runner/internal/runner/handoff_context.go:391-392`) unconditionally appends `[turn truncated due to handoff size limit]` whenever `assistant == ""` — while reporting `truncated:false`. A failed turn (no assistant text) is labeled "truncated", conflating "no assistant reply" with "truncated".

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom A**: `GET /provider-accounts` (and `/client/provider-accounts`) returns `"id":"acct-grok"`, `provider_key:"grok"`, `home_path:"/var/folders/…/TestRun20332…/001/grok-home"`, `slot_index:1`, `auth_status:"failed"` — a dead test record occupying grok slot 1 in the machine-global store shared by every runner instance.
- **Symptom B**: grok→codex handoff response (`l46-6-handoff-context-grok-source.json`) reports `includedTurnCount:4, omittedTurnCount:0, truncated:false`, yet `<previous_conversation>` renders `User:\nReply with exactly: ok\n[turn truncated due to handoff size limit]` — nothing was truncated; the turn simply has no assistant text (failed 402).
- **Expected**: tests isolate the provider-account store; the truncation marker only appears when `truncated:true`.
- **Impact**: low — a stale failed account pollutes account pickers/quota surfaces machine-wide; the marker misleads the receiving provider about conversation fidelity.

## Reproduction

- A: any runner on the machine → `GET /provider-accounts` shows `acct-grok` (cp46 `provider-accounts.json`, `client-provider-accounts.json`). Contrast legit accounts `506659be…`, `32a2460d…` (`.grokHome3`, active), `836d4840…`. To find the writer: grep tests for `acct-<provider>` id pattern / `SaveProviderAccounts` calls lacking env isolation.
- B: build any handoff where a source turn has `assistant == ""` (e.g., a failed turn) → response has `truncated:false` but the rendered turn ends with the truncation marker (`renderConversationTurn` → `appendTurnMarker` at `handoff_context.go:391-392`, marker text at :320, helper at :408).

## Root cause

- A (suspected): a test that seeds the provider-account store wrote through to the real user config dir instead of an isolated `FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH`/HOME — either a disk-backed `ListProviderAccounts`/`SaveProviderAccounts` call before env overrides, or a subprocess-level write bypassing the test env. `auth_status` flipping `connected`→`failed` on a later List pass is correct failure-marking of the stale record.
- B (confirmed): `apps/local-runner/internal/runner/handoff_context.go:391-392` —
  `if assistant == "" { return appendTurnMarker(sb.String(), truncatedMarker, maxBytes), false }` — unconditional append for assistant-less turns; returns `truncated=false`, so the envelope self-contradicts.

## Evidence

- `~/fp-beds/lt-evidence/cp46/BUG-LIVE-CP46-04-test-account-in-global-store.md`, `provider-accounts.json`, `client-provider-accounts.json`, `provider-accounts-defaults.json`, `accounts-context-grok.json`.
- `~/fp-beds/lt-evidence/cp46/BUG-LIVE-CP46-03-handoff-false-truncation-marker.md`, `l46-6-handoff-context-grok-source.json` (marker present, `truncated:false`, `omittedTurnCount:0`).

## Severity

- low

## Completion Notes (implemented 2026-09-23, CA-917)

- Fix A: `providerAccountsConfigPath` resolves an isolated
  `os.TempDir()/flowpilot-go-test/` path under `go test` when no
  FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH override is set — any forgetful
  test can no longer read/write the machine-global store. Explicit env
  override still wins.
- Fix B: `renderConversationTurn` no longer appends the truncation marker to
  assistant-less turns — the marker now appears only with `truncated:true`.
- Files: `provider_accounts.go`, `handoff_context.go`.
- Tests: `bug385_store_pollution_truncation_test.go` (path isolation under
  go test; no marker on empty-assistant turn; truncated flag intact).
