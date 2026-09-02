# BUG-337: TUI image send — You-box has no attach chip; Grok vision turn drops SSE so the answer never paints

## Metadata

- Document ID: `BUG-337`
- Title: `TUI image send — You-box has no attach chip; Grok vision turn drops SSE so the answer never paints`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-30`
- Last Updated: `2026-09-02`
- Feature Keys: `cli-tui`
- Parent Documents: [CP-56: Terminal TUI Chat And Flow Client](../../07-Coding-Plan/done/CP-56-Terminal-TUI-Chat-And-Flow-Client.md) (P-3b/D-10), [Task-319: Opencode Per-Model Image Capability](../../08-Task/done/Task-319-Opencode-Per-Model-Image-Capability.md), [CA-693: Opencode per-model image unlock](../../../change-audit/CA-693-opencode-per-model-image-unlock.md)
- Child Documents: `none`
- Related Documents: [CA-691: TUI step selected chip](../../../change-audit/CA-691-tui-step-selected-filled-chip-style.md), [CA-483: Grok image path fallback](../../../change-audit/CA-483-grok-image-path-fallback.md), [CA-537: No Thinking row] (app.go), [Task-282](../../08-Task/done/Task-282-TUI-Image-Attach-Via-Existing-Turn-API.md), [Task-319](../../08-Task/done/Task-319-Opencode-Per-Model-Image-Capability.md)
- Replaces: `none`
- Tags: `cli-tui, grok, opencode, image-attachments, sse, you-box, severity-high`

## AI Quick View

### Summary

- TUI: after sending a turn with images, the You-box shows only text — no `[N image attached]` chip. `ChatMessage` carries only `Role/Content/FormatHint`; `pendingAttach` is cleared before the box is rendered (`turn_stream.go:156`). Desktop already shows thumbs/chips (`Timeline.tsx:365`).
- Grok image turn `run-383622` (`grok-4.6`) `prompt="anh gi day"` + clipboard PNG via CA-483 path fallback → Grok did `read_file .tmp/images/turn-383624/0_clipboard.png` and answered ("Mèo con lông vàng cam…") per `run-383622-turns.ndjson` — but TUI showed only the initial delta `"Mình mở ảnh..."`, then silence. No `turn_completed` reached TUI; follow-up turns closed in ~120 ms with 0 events.
- `tui.log` proves the TUI stream stopped at `tool_completed` (22:45:52.598); runner `openStream` (`tui/client/client.go:1288`) uses `bufio.Scanner` 64 KB default, while `tool_completed.Output` carries the full PNG base64 (MB, via `grokToolOutput` `rawOutput.ImageContent`). `Scan()` fails, channel closes, `SendTurn` returns EOF as success → TUI settles `done` with no answer (CA-537 has no thinking placeholder to flag it).
- Opencode image path is NOT affected on image-send (native `{type:image,data:base64}` in `session/prompt`, Task-319) but shares the same `*ToolOutput` shape (`opencodeToolOutput` returns `rawOutput`/`content` verbatim) — same bomb if Opencode ever `read`s a PNG.

### Current Ask

- You-box must show an attach chip and SSE must stay alive past a huge tool payload. Stub oversized image bytes at the mapper so the SSE frame is small; bump the TUI scanner to 8 MB; surface EOF without a terminal event as an error, not `done`.

### Key Decisions

- `V-1` Stream must not settle `done` on silent EOF — EOF without `turn_completed`/`turn_failed` is an error.
- `V-2` You-box chip wording matches the operator's ask: `[1 image attached]` / `[N images attached]`, rendered outside the 4-line clamp so it is always visible.
- `V-3` Tool payload bytes are never shown in the TUI — stub at the mapper (`[image 12345 bytes omitted]`) so SSE and logs stay small.
- `V-4` Scanner bump is agnostic (one client path for all providers); image stub is shared `grok` + `opencode` (proven same shape).

### Constraints

- Additive-tests-only — no edits to green pre-existing tests; new files only.
- Cross-provider parity: F-2/F-3 agnostic (one path); F-1 shared Grok+Opencode; F-4 agnostic — cite evidence (this doc).
- Keep Desktop thumb/chip untouched; keep CA-537 "no thinking row".
- Out of scope: first-send `409 session_unavailable` (22:45:11) and Desktop image paste.

### Open Questions

- Persisting attach names into replay/durable history so `/open` repaints the chip is not done in v1 (old events lack the names) — acceptable gap (same as Desktop pre-Task-052 history).

### Source Refs

- `run-383622` (`/Users/tiendat/Desktop/flowpilot/flowpilot/.flowpilot/runs/db51ec26-1a0f-4b92-8ceb-b03dc8e9b363/run-383622`), `run-383622-turns.ndjson` (3 prompt/transcript_turn pairs), `tui.log` (22:45:40 turn_started … 22:45:52 tool_completed → closed), `grok_attachments.go` (path fallback), `grok_adapter.go:332`, `grok_event_mapper.go:415` (`grokToolOutput`), `opencode_event_mapper.go:225`, `opencode_acp.go:94`, `tui/client/client.go:1288` (`openStream`), `tui/app/turn_stream.go:131,156`, `tui/app/app.go:3362`, `tui/app/model.go:38` (`ChatMessage`), `tui/app/chat_box.go:317` (`youBox`).

## 1. Issue Summary

On the TUI, sending a chat turn with a clipboard/paste image leaves no visual indication on the sent prompt that an image was attached — the You-box renders only the text. The operator requested at minimum a chip/flag such as "[1 image attached]". Separately, a Grok vision turn that does carry an image (`run-383622`, `grok-4.6`) rendered only the very first `message_delta` (`"Mình mở ảnh..."`) and the `→ read_file` tool indicator, then the stream silently ended; the Grok answer describing the cat (and the `turn_completed`) never appeared in the TUI (runner-side transcript does contain it).

## 2. Parent Links

- impacted coding plan: `CP-56 P-3b/D-10` (TUI image attach)
- impacted tech design: `SD` image attachment contract (Task-052/TASK-282 wire, Task-319 native blocks)
- impacted system spec: n/a (TUI client delta)

## 3. Environment and Reproduction

- environment: TUI `flowpilot chat` against local runner, provider `grok-4.6` (and any provider for the chip). `opencode 1.18.25`, Grok ACP 0.2.x, commit `a2f96c5c`.
- reproduction steps:
  1. `flowpilot chat` with project `Gate-sandbox` (`db51ec26`), provider `grok-4.6`.
  2. Copy an image to clipboard, `Alt+V` (or `/image` + path) — verify `[N img]` badge on the composer (`renderInputLine`).
  3. Send a short text with the image (e.g. `"anh gi day"` + 1 clipboard PNG). Observe the You-box.
  4. Observe the stream: first delta arrives, `read_file` tool appears, then no further deltas/answer.
- frequency: deterministic for Grok vision turns that produce a `read_file` on the queued image; chip missing is every image turn for every provider.

## 4. Expected vs Actual

- expected:
  - You-box contains a visible chip such as `[1 image attached]` (or `[N images attached]`) in addition to the prompt text, not truncated by the prompt clamp.
  - Grok answer describing the image appears and the turn reaches `turn_completed` (`done`).
- actual:
  - You-box contains only the prompt text — no chip, no image count.
  - TUI stops after `tool_completed` (22:45:52.598, `tui.log`), shows no cat description and never receives `turn_completed`; the run directory and `run-383622-turns.ndjson` do contain the full answer (so the runner did complete the turn).

## 5. Impact

- users affected: any TUI user sending images (operator)
- workflows affected: chat image attach for grok (vision via path fallback) and all-provider You-box verification
- severity: high — image send appears to vanish on the TUI; Grok vision hangs without error are a blocker for QA

## 6. Root Cause

- hypothesis: large tool payload on SSE kills the stream and no chip was ever added to `ChatMessage`.
- confirmed cause:
  1. `openStream` (`apps/local-runner/internal/tui/client/client.go:1288`) uses `bufio.Scanner` with its 64 KB `MaxScanTokenSize` default. `grokToolOutput` (`apps/local-runner/internal/runner/grok_event_mapper.go:415`) returns the raw `rawOutput`/`content` verbatim, which for a `read_file` on the Grok fallback PNG includes `ImageContent: {data: "<base64 PNG>", mimeType:"image/png"}` (MB). The serialized `ProviderEvent` on a single `data:` line exceeds 64 KB → `scanner.Scan()` fails → the TUI stream closes silently.
  2. `SendTurn` (`tui/client/client.go:1144-1160`) treats an EOF-closed stream that never saw `turn_completed`/`turn_failed` as success (closes without sending on `errCh`). `app.go:1327` therefore closes `turnStream` as `Err==nil` → `status=done` (CA-537 moreover leaves no thinking placeholder, so "no assistant text" is not surfaced).
  3. You-box: `ChatMessage` (`tui/app/model.go:38`) has no attachment field; `m.addMessage("user", input, "")` (`app.go:3362`) copies only the text and `turn_stream.go:131,153-156` retains base64 in `attachments` only to POST then nils `pendingAttach` — the names are not transferred to the message. `youBox` (`tui/app/chat_box.go:317`) has no chip path. Desktop already has `PromptAttachmentView` + chip (`Timeline.tsx:365`, `store.ts:1449`).
  4. `opencodeToolOutput` (`apps/local-runner/internal/runner/opencode_event_mapper.go:225`) has the identical `return out` shape and would produce the same bomb if an Opencode `read_file` on a PNG were ever done (not on image-send today, which uses native ACP image blocks per Task-319).
- evidence: `tui.log` 22:45:40 `turn_started` → 22:45:51 message deltas → 22:45:52 `tool_completed` → 22:45:52 `turnStreamClosedMsg{nil}`; `run-383622-turns.ndjson` shows transcript_turn with cat description (runner completed); `grok_attachments.go` writes 0_clipboard.png; `grok_adapter.go:332-341` path fallback + `read_file`; `.flowpilot/runs/.../run-383622/turn-*-params.json` (`grok`, `grok-4.6`).

## 7. Fix Strategy

- `F-1` **Stub oversized image bytes at the mapper** — `grok_event_mapper.go:415` (`grokToolOutput`) and `opencode_event_mapper.go:225` (`opencodeToolOutput`): if `rawOutput`/`content` contains an image payload (`ImageContent`, `type:image`, base64 field `_data`/`data`), return a stub such as `{"type":"image_omitted","bytes":N,"hint":"[image N bytes omitted]"}` (or a short string `[image N bytes omitted]`). Short text outputs remain unchanged. **Grok + Opencode share this shape** (grep proves both return verbatim); Codex (`codex_event_mapper.go:72,97` `paramAny(..., "output")`/item) and Claude (`block["content"]`) use different shapes — read their call sites and do not change them unless the same image bomb is proven.
- `F-2` **Bump TUI SSE scanner buffer (agnostic)** — `tui/client/client.go:1288`: `scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)` (same as `grok_process.go:121` / `opencode_process.go:112`, etc.). One client path for all providers.
- `F-3` **Treat EOF without a terminal event as failure (agnostic)** — `tui/client/client.go` `SendTurn`: if the filtered stream ends without seeing `turn_completed`/`turn_failed` for the posted `turnID`, send an error on `errCh` (e.g. `turn stream ended without terminal event`). `app.go:1327` `/open` errors already surfaces as `system` error + `turn failed`; no silent `done`.
- `F-4` **You-box attach chip (agnostic)** — `tui/app/model.go:38`: add `Attachments []string` (original names, not base64) to `ChatMessage`. In `app.go:3362` capture `originalNames := make([]string,len(m.pendingAttach))` before `openTurnStream`/`clearPendingAttachments`. In `chat_history_replay.go`/`history.go` carry through on replay when present (nil for old events — no chip, same gap as Desktop pre-052). In `chat_box.go:317` (`youBox`): render a chip line such as `│ [1 image attached]                                  │` (or `[N images attached]`) **outside** the `maxUserPromptLines` clamp so it is always visible; reuse `youBox` row styling, ASCII-safe, unit-tested with `WindowSizeMsg` mocks — not a regression guard edit.

## 8. Validation

- `V-1` Grok + clipboard PNG: TUI shows `[1 image attached]` on the You-box and the full Grok cat description arrives (`turn_completed` observed) — `tui.log` contains the deltas and `turn_completed`, not just `tool_completed`.
- `V-2` Zero images: You-box has no chip (no false positive).
- `V-3` Multi-image: You-box shows `[N images attached]` with the exact file names in the chip (sorted by attach order) — additive test.
- `V-4` `409 session_unavailable` on a first send is out of scope and must not be conflated with SSE truncation.
- `V-5` `grokToolOutput`/`opencodeToolOutput`: image payload → omitted stub, text-only payload → unchanged (unit).

## 9. Regression Guard

- tests: **additive only** — new files (e.g. `tui/client/sse_large_frame_test.go`, `runner/grok_image_output_omit_test.go`, `runner/opencode_image_output_omit_test.go`, `tui/app/you_box_image_chip_test.go`), no edits to green legacy tests. Old fail → STOP + report per oracle-rule.
- alerts: scanner change is agnostic; image stub is provider-specific (Grok + Opencode) by evidence — document the evidence if Claude/Codex are left unchanged.
- audit checks: no ledger rework; this bug does not touch durable replay (durable-replay-contracts matrix: no new resume ordering).

## 10. Follow-Up Document Updates

- upstream docs that must change: `CP-56 P-3b/D-10` if You-box visual is considered the spec for image turns.
- notes left unchanged on purpose: `CA-483` (Grok path fallback design), `CA-693` (Opencode native image), `CA-537` (no thinking row) — this fix extends them, not replaces.
