# CA-694 — TUI image chip + Grok SSE drop fix (BUG-337)

## Problem

1. **You-box missing chip** — TUI `ChatMessage` had only `Role/Content/FormatHint`. After `SendTurn` the `pendingAttach` base64 was copied to `TurnInput.Attachments` and then nilled (`turn_stream.go:156`), but the names were never copied to the rendered `ChatMessage`. `youBox` rendered only the text, so an image send looked like a text-only send. Desktop already had `PromptAttachmentView` chips (`Timeline.tsx:365`, `store.ts:1449`).

2. **Grok vision turn silent on TUI** — `run-383622` (`grok-4.6`) sent `"anh gi day"` + clipboard PNG via the CA-483 path fallback (`.tmp/images/turn-383624/0_clipboard.png`). Grok did `read_file` on that PNG and answered ("Mèo con lông vàng cam…") — runner transcript `run-383622-turns.ndjson` and `tui.log` prove it. TUI showed only the initial `message_delta` (`"Mình mở ảnh…"`) and the `→ read_file` tool row, then the stream closed at `tool_completed` (22:45:52.598) with **no `turn_completed`** and no cat description. Follow-up turns closed in ~120 ms with 0 events.

   Root cause was two-fold:
   - `grokToolOutput` (`grok_event_mapper.go:415`) returned the raw `rawOutput.ImageContent {data: "<base64 PNG>", mimeType}` verbatim. The serialized `ProviderEvent` on a single `data:` SSE line was MB-sized.
   - `openStream` (`tui/client/client.go:1288`) used `bufio.Scanner` with its **64 KB** `MaxScanTokenSize` default. `Scan()` failed on the huge line, the loop exited, `SendTurn` closed both channels without sending on `errCh` (EOF without a terminal event was treated as success), and `app.go:1357` settled `status=done`. CA-537 has no thinking placeholder, so no "no assistant text" banner appeared.
   - `opencodeToolOutput` (`opencode_event_mapper.go:225`) had the identical `return out` shape and would produce the same bomb if Opencode ever `read_file`d a PNG (image-send today is native ACP blocks per Task-319, so it didn't hit the same path, but the shape is proven shared).

## Change

- **Runner — stub oversized image bytes at the mapper (F-1, Grok + Opencode, shared shape)** — `grok_event_mapper.go:415` and `opencode_event_mapper.go:225` now call `omitOversizedImageOutput` helpers. If `rawOutput`/`content` contains `ImageContent` with base64 `data`, or a `data`+`mimeType:image` with large base64, or any string/map with >64–100 KB image payload, the helpers return a short stub such as `"[image omitted: 123456 bytes]"` or `"[output omitted: …]"`. Short text outputs pass through unchanged. Helpers are provider-specific file-local functions (`bug337Grok*` / `itoaOpencode*`) to avoid collisions with existing `itoa`/`containsFold` helpers.

- **TUI SSE — bump scanner buffer (F-2, agnostic)** — `tui/client/client.go:1288` now `scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)` (same as `grok_process.go:121` / `opencode_process.go:112`). One client path for all providers — Codex/Claude/Grok/Opencode plus any future provider.

- **TUI SendTurn — EOF without a terminal event is an error (F-3, agnostic)** — `tui/client/client.go:1144` now tracks `seenTurnEvent` (any event with `ProviderTurnID == turnID`) and `seenTerminal` (`turn_completed`/`turn_failed` for that turn). If the SSE stream ends (`range openStream` exits) without a terminal **and** at least one turn event was seen, `errCh <- "turn stream ended without terminal event"` is sent. Context cancellation is not surfaced as an error. Empty streams (no turn events, e.g. `TestA1_9`) remain silent success, preserving the existing test contract. `app.go:1327` already renders `turnStreamClosedMsg.Err` as a `system` error + `turn failed`.

- **TUI You-box — attach chip (F-4, agnostic)** — `tui/app/model.go:38` adds `Attachments []string` to `ChatMessage`. `tui/app/app.go:3362` captures `OriginalName` from `pendingAttach` into the just-added user message before `recordPromptHistory`. Rendering (`tui/app/app.go:4926`) appends a chip line `[1 image attached]` / `[N images attached]` **after** the 4-line prompt clamp, so the chip is always visible even on a long prompt (and survives `userPromptExpanded` toggling). No base64 is stored in the message — only names.

## Tests (additive, no pre-existing edits)

- `runner/bug337_grok_image_output_omit_test.go` (4) — Grok: ImageContent, data+image mime, large string omitted; text preserved.
- `runner/bug337_opencode_image_output_omit_test.go` (3) — Opencode: same matrix (shared shape).
- `tui/client/bug337_sse_large_frame_test.go` (2) — Agnostic: 200 KB `tool_completed` frame survives (8 MB buffer) + terminal; EOF without terminal → error, empty stream → no error (preserves `TestA1_9`).
- `tui/app/bug337_you_box_image_chip_test.go` (5) — Agnostic (matrix over codex/claude/grok/opencode): 0 images → no chip, 1/2 images → correct wording, chip outside clamp (4-line long prompt), chip persists after `pendingAttach` cleared, end-to-end `pendingAttach → You-box`.

All new tests green:
- `go test ./internal/runner -run TestBug337` — ok
- `go test ./internal/tui/client -run TestBug337` — ok
- `go test ./internal/tui/app -run TestBug337` — ok
- `go test ./internal/tui/client -count=1` — ok (36 s, includes `TestA1_9` empty-stream preservation)
- `go test ./internal/tui/app -count=1` — ok (7 s)

## Parity classification (R2)

- **F-2 + F-3 + F-4** — **provider-agnostic**: one `openStream`/`SendTurn`/`ChatMessage`/`youBox` path, grepped for `ProviderKey`/`SupportsImages` — no branching. Evidence: `rg -n "ProviderKey|SupportsImages" tui/client/client.go tui/app/app.go tui/app/model.go` shows `openStream`/`SendTurn` never branch on provider, `youBox` chip is unconditional on `msg.Attachments`. One suite covers all providers; the chip test matrices over codex/claude/grok/opencode anyway.
- **F-1** — **shared pattern, per-adapter wiring** — both Grok and Opencode mappers return `rawOutput`/`content` verbatim (`grokToolOutput` / `opencodeToolOutput` — grep in `grok_event_mapper.go`/`opencode_event_mapper.go` proves identical shape). Codex (`codex_event_mapper.go:72,97` `paramAny(..., "output")`) and Claude (`claude_event_mapper.go:115` `block["content"]`) use different tool-output shapes — left unchanged and explicitly documented as "not proven to carry ImageContent on image-send" (native multimodal). Adding Grok+Opencode stubs does not risk Codex/Claude regression.

## Will not undo

- CA-483 (Grok `.tmp/images` path fallback) — extended, not replaced (the PNG still lands on disk; only the SSE payload is stubbed).
- CA-693 (Opencode native `{type:image}` blocks) — image-send still native; the `read_file` fallback bomb is now stubbed for Opencode as well as Grok.
- CA-537 (no thinking row) — chip is appended to `lines` after clamping, not as a thinking placeholder.
- Task-282 / Task-319 per-model gate — `supportsVisionFor` / `ValidateAttachmentsForModel` still gate whether images can be queued; the You-box chip only reflects what was actually queued.

## Out of scope

- First-send `409 session_unavailable` at 22:45:11 (separate session-store race).
- Persisting chip names into durable history / `/open` replay (old events lack the names) — same gap as Desktop pre-Task-052.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-337
change_type: bugfix
summary: Keep TUI image-turn SSE alive (stub image bytes in Grok/Opencode tool output, 8 MB scanner) and show [N image attached] chip on You-box
# --->8---
