# CA-709 — Opencode array content for agent_message_chunk (BUG-341)

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-341
change_type: bugfix
summary: handle array and output_text shapes for agent_message_chunk so tool-heavy first turns are not blank (433929)
# --->8---

## What changed

- `opencode_event_mapper.go` `opencodeTextContent`: now handles `string`, `[]any` (array of `{type:"text",text}` parts), and `output_text`/`content` types. Previously only `map` with `type=="text"` was accepted, so a chunk shaped as `[{type:"text",text:"..."}]` or `{type:"output_text",text:"..."}` was dropped → `lastText` stayed `""` → even the 8s wait had nothing to grow and `turn_completed` stayed blank (433929). Simple `hi` prompts use single-map text and worked.
- No change to `drainOpencodeNotificationsBlocking` logic (CA-708); this is a complementary mapper fix for the same blank.

## R1 evidence

- New additive tests (no legacy edits), all PASS:
  - `TestOpencodeTextContentArray` — `content: [{type:"text",text:"Chao "},{type:"text",text:"Nam!"}]` → `Chao Nam!`
  - `TestOpencodeTextContentOutputText` — `type:"output_text"` → `hello output`
  - `TestOpencodeArrayThenTextViaBlockingDrain` — array chunk via `drainBlocking` → `array answer` (≈150ms)
- Old suites still green:
  - `go test ./internal/tui/app -count=1` 7.36s PASS (CA-705 `turnLive` still green)
  - `go test ./internal/runner -run TestOpencode -count=1` PASS (all 20+ opencode tests, plus the 5 starvation/late tests)
  - `TestOpencodeUsageUpdateDoesNotStarveText` etc. still PASS (0.55s each)
- Provider parity: opencode-only wire shape. The fix is in the opencode mapper only; Claude/Grok send `type:"text"` single-map and are unaffected. The runner-side `turn_completed` with text now benefits both TUI and Desktop (same `sendTurn` filter), so no Desktop `store.ts` change needed — verified by the same runner fix.

## Honest gaps

- Other `sessionUpdate` kinds that might carry text (e.g. future `agent_message` without `_chunk`) are not yet mapped; they would still be dropped and need a new live capture.
- TUI empty-terminal hydrate fallback is still TODO (same as CA-707/708).

## Prior CA not undone

- `CA-708` wait-for-text only on text growth, `CA-707` 8s vs 150ms conditional wait, `CA-705` TUI `turnLive` + C2 guard, and `CA-696..CA-702` chat-history contracts remain. This extends the mapper to handle the wire shape that made even the 8s wait produce no text.
