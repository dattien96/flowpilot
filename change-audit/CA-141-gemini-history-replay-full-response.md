# CA-141: Gemini History Replay Full Response

## Summary

Fixed Gemini chat-history replay after runner restart so reopened Gemini chats use durable turn-level prompt/assistant pairs instead of mismatching prompt sidecar entries with the latest standalone response or the truncated `LastMessage` history summary.

## What Changed

- Extended the local per-run turn-log sidecar with a `transcript_turn` entry type that stores one visible Gemini prompt and assistant response together.
- Persisted raw prompt entries with the provider turn id, then used that id during Gemini replay to backfill assistant-only `transcript_turn` entries with the correct prompt instead of drifting every response toward the latest prompt.
- Persisted Gemini assistant text from `message_completed` before falling back to `turn_completed.final_message`, because the final message can be a shortened UI summary.
- Updated Gemini transcript rehydration to prefer complete turn-level entries, preserve prompt-only legacy sidecar history, and attach any legacy standalone response only to the latest unmatched prompt.
- Stamped synthetic Gemini replay events with distinct per-turn `ProviderTurnID` values so the desktop timeline reducer does not collapse multiple replayed prompts into the same `prompt-undefined` identity.
- Forced the desktop composer clear to flush synchronously and remount the textarea before async send work begins, and fixed the `@agent` busy-feedback send path so it clears consistently after sending.
- Added regression coverage proving Gemini restart replay restores multiple prompt/answer turns in order with stable per-turn ids, assistant-only turn entries recover the correct prompt by turn id, full message text wins over a shortened final summary, and partial old sidecars keep all prompt history instead of collapsing to only the latest turn.

## Verification

- `go test ./internal/runner -run 'TestResumeRunGeminiPersistedResumeRehydratesFromProjectConfig|TestGeminiTranscriptFallbackPairsPartialSidecarFromEnd|TestGeminiTranscriptTurnsBackfillsPromptsByTurnID|TestGeminiTranscriptCapturePrefersFullMessageCompleted|TestTurnLogStoreRoundTrip|TestResumeRunInMemoryCompletedGeminiRemainsReadable' -count=1`
- `go test ./internal/runner -count=1`
- `npm --prefix apps/desktop-flowpilot run typecheck`

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: Task-167
change_type: bugfix
summary: Persist Gemini prompt/assistant turn pairs for restart history replay
# --->8---
