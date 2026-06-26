# CA-132 — Prompt Context Continuity and Cross-Provider Handoff

This session delivered the prompt-context continuity stack end to end:

- `apps/local-runner/internal/changeledger/*`
  - Feature-key validation now treats registry-verified keys as authoritative and keeps unknown brackets low-confidence.
  - CA notes now contribute a bounded `Scope` / `Residual Notes` excerpt for recent history entries.
  - Chat-summary persistence was added so per-feature discussion history can be injected later.
- `apps/local-runner/internal/featurecatalog/*`
  - Feature catalogs now infer file-glob hints from ledger history and can rank candidates from changed paths.
  - History slots now render bounded CA excerpts and chat-summary timelines in the same extensible history block.
- `apps/local-runner/internal/flowgate/*`
  - A new `commit_feature_key_missing` reprompt gate now pushes the AI toward registry-verified keys instead of letting typos silently fragment history.
- `apps/local-runner/internal/runner/*`
  - Prompt assembly now injects per-feature history before the model sees the turn.
  - Feature resolution is conversation-sticky (`resolveTurnsFeature`): it scans user prompts newest→oldest, so a low-signal continuation/retry prompt ("continue", "try again") inherits the established feature for both injection and chat-summary recording, while an explicit pivot re-resolves. The same helper backs the handoff summary lookup.
  - The chat-summary `state_key` is now a SHA-256 of the run transcript (was the raw concatenated text), keeping the ndjson key O(1) in size while still refreshing on any content change.
  - Chat-summary generation moved off the per-turn path to three triggers: a 5-minute idle timer (reset by any new turn), a manual `POST /client/workflow-runs/{runId}/chat-summary` endpoint (generate-now, 409 while running, no-op on hash match), and a one-shot startup background scan that backfills chats missing a summary or with a stale hash. Summaries are now upserted one line per (run, feature) instead of appended per turn, and each feature's summary is built only from that feature's turns (per-feature bucketing — no cross-feature mixing).
- `apps/desktop-flowpilot/src/*`
  - Added a "Gen summary" button in the chat controller near the YOLO toggle (enabled only when the chat is idle/completed), a `generateChatSummary` client method + store action, and the `ChatSummaryResult` contract type.
  - The runner now serves bounded handoff context for cross-provider chat switching and records chat summaries after completed turns.
  - Chat-summary generation now uses a real cheap-tier model of **the chat's own provider** (`SummarizeChatTranscript` — one-shot exec via `resolvePromptExecutionAdapter`: Claude `--print haiku`, Codex `exec`, Gemini `gemini-2.5-flash`; overridable via `FLOWPILOT_SUMMARIZER_MODEL[_<PROVIDER>]`), with a deterministic keyword-heuristic fallback when no account/CLI is reachable, a `state_key` cache so an unchanged transcript skips regeneration, and a goroutine so the model call never blocks turn finalization. This replaces the original heuristic-only stopgap (CP-37 Task-161/162 summarizer decision).
- `apps/local-runner/internal/contextsync/*`
  - Engine context sync now includes `ledger/chat_summary.ndjson` so CP-37 per-feature discussion summaries travel with the other shared context files.
- `apps/local-runner/internal/runner/engine_drive_sync.go`
  - The runner now reuses one context-engine sync helper from engine init and after successful chat-summary append.
- `apps/desktop-flowpilot/src/*`
  - The desktop now confirms provider switches, requests handoff context, starts a new run under the target provider, and sends the bounded handoff prompt as the first turn.

## Why

The goal was to make both sides of context continuity trustworthy: code-history context should resolve to the right feature and carry the CA "why", and provider switching should preserve the visible conversation without mutating the source run. The desktop UI and runner backend now share the same bounded-context contract.

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-157
change_type: feature
summary: wire feature-history injection, bounded chat summaries, and cross-provider handoff
# --->8---
