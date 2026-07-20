# CA-379 — run-20332 history replay parity

## Summary

Restored the replay contract for completed multi-turn flow hubs: reopening a
history item loads its provider-owned transcript for Codex, Claude, and Grok,
then uses durable `transcript_turn` rows only to fill a response missing from a
provider file. Grok no longer takes a flow-hub-only early return that replaced
`chat_history` with a partial turn log and could reopen older runs with an empty
main transcript.

The terminal replay event remains explicit so the shared desktop reducer clears
the stale `Thinking...` affordance. Flow history status is resolved from the
terminal loop state rather than which history item happens to be selected.

## Cross-provider parity

Classification: shared replay contract with provider-specific transcript
loaders. `seedTranscriptFromDisk` dispatches to the Codex, Claude, and Grok
loaders; each was exercised by the new two-turn flow-hub restart matrix.
`historyStatusForLiveRun` and the desktop terminal reducer do not accept or
branch on `ProviderKey`; the new desktop test nevertheless runs the same event
sequence for all three keys.

## Verification

- `go test ./internal/runner -count=1 -run 'TestRun20332|TestRun12613|TestRun2334RestartReplayKeepsAssistantResponsesForEveryProvider|TestRun2334NormalGrokRestartReplayUsesRawPromptsAndTurnAnchors|TestSeedTranscript|TestRun1264Restore'` — 26 passed after the turn-id fallback addition.
- `npm --prefix apps/desktop-flowpilot run build` — passed.

## Durable replay contract matrix

| Surface | Evidence | Codex / Claude / Grok |
| --- | --- | --- |
| Durable transcript | Provider history is primary; a durable assistant fallback is keyed by turn id and cannot collapse equal response text. | `TestRun20332FlowHubHistoryParityForEveryProvider`, `TestRun20332TurnLogFallbackPreservesDuplicateResponseOrderForEveryProvider` |
| Event order | A missing earlier response is inserted before the next turn. | turn-id fallback matrix |
| Interactive state | Existing sidecar anchoring regression remains in the selected runner battery. | `TestRun2334*Anchors*` |
| Agent lifecycle | Existing restored flow lifecycle coverage remains in the selected runner battery. | `TestRun1264Restore*` |
| Terminal state | Resume ends with `turn_completed`; desktop reducer removes `Thinking...`. | runner parity matrix; `timeline_terminal_parity.test.ts` type-checked by build |

Unexercised: live desktop click-through against the three authenticated provider accounts remains manual E2E work; this change verifies the persisted restart boundary only.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: bugfix
summary: Preserve provider transcript parity for multi-turn flow history replay and clear terminal loading state across Codex, Claude, and Grok.
# --->8---
