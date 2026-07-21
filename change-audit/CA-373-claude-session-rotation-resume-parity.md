# CA-373: Claude multi-turn session no longer rotates (resume parity)

## Summary

Fixed BUG-295: a multi-turn Claude interactive run silently rotated its provider session after turn 1, so restart replay (which loads the single persisted session file) lost every turn after the first. Root cause was a **regression** from commit `b288982` (Grok cross-account relocate, CA-312), which broadened a Codex-only override in `runTurn` (`if rs.providerKey == ProviderKeyCodex && rs.realProviderSessionID != ""` → `if rs.realProviderSessionID != ""`) so the REAL session id is fed to ALL providers. Codex/Grok/Gemini adapters consume that id directly (or tolerate it); Claude's adapter alone treats `req.ProviderSessionID` as the SYNTHETIC pool KEY, so the broadened predicate made `pool.realSession(realID)` miss, dropped `--resume`, and started a fresh Claude session. Claude was collateral — never in b288982's intent.

Two complementary additive fixes:

- **F-3 (service-gate, the precise regression inverse)** — `interactive_service.go` `runTurn`: `if rs.realProviderSessionID != "" && rs.providerKey != ProviderKeyClaude`. Claude live turns keep passing the synthetic pool key (pre-regression pool-mapped resume restored); Codex/Grok/Gemini untouched, so the b288982 Grok cross-account fix survives intact.
- **F-1 (adapter safety net)** — `claude_adapter.go` `SendTurn`: when the pool lookup misses AND `req.ProviderSessionID` is already a real id (not synthetic `thread-*`), resume it directly. Covers the after-restart case where reconstruct seeds `providerSessionID` from the persisted REAL id (`interactive_resume.go:833-834`) into a fresh, empty pool — which F-3 alone cannot fix.

F-3 handles the live follow-up turn; F-1 handles the first new turn after a restart. Together they prevent the rotation on every path that hands the adapter a session id.

## Verification

- `go test ./internal/runner -run TestBug295 -count=1`: 5 passed (3 F-3 provider-matrix: Claude keeps synthetic, Codex + Grok keep real; 2 F-1: real-id pool-miss resumes, synthetic pool-miss does not).
- Regression battery (`TestClaudeAdapter*`, `TestRun2334RestartReplayKeepsAssistantResponsesForEveryProvider`, `TestChatModeSelectedControlsReachProviderTurnRequest`, `TestLiveChatInjectsFeatureHistoryForSupportedProviders`): 20 passed. Pre-existing `TestClaudeAdapterResumeUsesRealSessionID` stays green unmodified (additive-tests-only).
- Broader-suite residual failures (Codex-CLI resume, Grok home slot, skills merge) reproduce identically on a stashed baseline — pre-existing/environmental (no Codex CLI binary; machine home paths), none in the Claude-adapter/resume area.
- `-race` not run: this machine lacks gcc/CGO.
- GitNexus impact analysis unavailable (`%1 is not a valid Win32 application`); localized tracing confirmed b288982 broadened only the `runTurn` consume site (not `sessionStateOf`), that only `claude_adapter.go` lacks a real-id fallback, and that reconstruct seeds the real id.

## Files

- `apps/local-runner/internal/runner/interactive_service.go`: F-3 — exclude Claude from the `runTurn` real-id promotion.
- `apps/local-runner/internal/runner/claude_adapter.go`: F-1 — resume a real id directly on a pool miss (+ `strings` import).
- `apps/local-runner/internal/runner/bug295_claude_session_rotation_test.go`: additive regression + non-regression tests (Claude/Codex/Grok).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-295
change_type: bugfix
summary: Claude multi-turn runs keep the synthetic pool key (Codex/Grok still get the real id) and the adapter resumes a real id on a pool miss, so sessions no longer rotate and restart replay keeps full history.
# --->8---
