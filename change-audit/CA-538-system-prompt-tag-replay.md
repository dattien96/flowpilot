# CA-538 — [SYSTEM_PROMPT] tag for durable internal-prompt classification on replay

## Summary

Fixed BUG-326, found live on run-104296: a flow gate reprompt that stopped a
run mid-question was persisted by Grok wrapped inside a `## History …` preamble
plus a `grokAskUserReinforcement` `<user_query>` block, so on restart the shared
overlay treated it as the user's own next question and dropped the genuine
follow-up.

Root cause: internal-vs-user classification relied on legacy `HasPrefix`
detection of bare gate/flow-engine/handoff sentences (`isSystemPrompt` /
`isGateReprompt` / `isFlowEnginePrompt`), which cannot survive provider wrapping.

Fix — a durable tag as the single source of truth for system prompts:

1. `feature_history.go` gains `const systemPromptTag = "[SYSTEM_PROMPT]"`.
2. `isSystemPrompt` checks the tag first, then falls back to the legacy
   detectors so old runs recorded before the tag still replay correctly.
3. `isGateReprompt` switches from `HasPrefix` to `Contains` so a wrapped gate
   sentence is recognized even without the tag.
4. `isFlowEnginePrompt` becomes tag-aware so direct callers (e.g. `skipHistory`)
   classify consistently.
5. `interactive_service.go` `startTurn` stamps `[SYSTEM_PROMPT]\n` last-mile,
   immediately before the prompt is emitted / appended to the turn log / handed
   to `runTurn`. The tag lands on exactly what the provider receives and what the
   turn log records, so live and replay classify identically. The stamp is
   idempotent (skips already-tagged prompts) and is never applied to user prompts.

## Cross-provider parity

Classification: **Case 1, shared.** `isSystemPrompt` is provider-agnostic; no
per-provider branch was added. The shared matrix tests
(`TestRun104296WrappedGateReplayKeepsQAPairing`,
`TestSystemPromptTagStampedPromptsAreSystem`,
`TestLegacySystemPromptDetectorsProviderParity`,
`TestOverlayRawTurnPromptsWrappedGateSlotsDirect`) exercise the shared code
paths that Claude, Codex, and Grok all route through. Tag-stamping happens in
the shared `startTurn`, before any provider adapter sees the prompt.

## additive-tests-only compliance

Only a new test file was added
(`run104296_system_prompt_tag_replay_test.go`); no existing test file was
modified. The legacy detectors are intentionally retained (not deleted), so
pre-tag persisted runs keep their current behavior.

## Verification

- New tests green: `ok flowpilot-runner/internal/runner 1.921s` (after a
  one-off fixture correction in `TestOverlayRawTurnPromptsWrappedGateSlotsDirect`
  where the gate text was being prepended to every slot instead of only the gate
  slot).
- Prior overlay/replay/gate/feature battery all green with the fix (12.055s):
  run2334, bug083, bug293, bug306, run1264, bug295, bug289, gate, hub, durable
  resume, flow-engine, handoff suites.
- Full-suite `go test ./internal/runner/ -count=1`: the only failure is
  `TestGeminiAdapterPromptPrepAndEnv` (`context deadline exceeded` while the
  external `agy` binary prints) — re-run in isolation it passes (2.023s); it is
  a pre-existing environment flake unrelated to this change.
- `go build ./...` and `go vet ./internal/runner/` clean; `gofmt` clean for all
  changed files (feature_history.go, interactive_service.go, new test file).

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: BUG-326
change_type: bugfix
summary: Durable [SYSTEM_PROMPT] tag stamped last-mile on every internal prompt so replay classifies provider-wrapped gate reprompts as system (not user), with tag-first detection in isSystemPrompt and legacy-detector fallback plus Contains-based isGateReprompt for pre-tag runs.
# --->8---
