# CA-285: Grok Handoff Source + Chat Summary Skip Reasons

## Scope

Task-212 remaining: (1) Gen summary opaque “unchanged or no feature” reason; (2) Grok → Codex handoff blocked as unsupported source.

## Root Cause

- Summary: `generateChatSummaryNow` mapped every `recordChatSummarySync=false` to one string; ledger is feature-scoped so unresolved features cannot write.
- Handoff: `supportsHandoffSource` only Claude/Codex despite `loadGrokTranscriptEvents` / `seedGrokTranscriptFromDisk` (DOD-9).

## Changes

- `chat_summary.go`: `chatSummaryResult` statuses + precise `reason`; seed Grok transcript when events empty on manual gen.
- `handoff_context.go`: `supportsHandoffSource` includes `ProviderKeyGrok`.
- `store.ts`: “Chat summary skipped (reason)” when skipped with reason.
- Tests: Grok summary resolvable/no-feature/already-current; Grok→Codex live handoff.

## Verification

- `go test ./internal/runner -run 'GenerateChatSummaryNow|BuildHandoffContext'` — PASS
- Live: restart runner; Gen summary with feature keyword; Start new chat with Codex from Grok

# ---8<--- flowpilot:change-ledger
feature_key: cross-provider-handoff
source_doc_id: Task-212
change_type: bugfix
summary: Enable Grok as handoff source via chat_history.jsonl; split chat summary skip reasons for Gen summary
# --->8---
