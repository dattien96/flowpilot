---
id: CA-485
feature_key: cli-tui
title: Grok message chunks no longer dropped — TUI shows answer after image turns
date: 2026-08-14
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-483-grok-image-path-fallback
will_not_undo: path fallback; Capabilities.Vision=false; image cleanup after turn
```

## Issue (run-96217 + image.png)

Operator attached 2 images on Grok; runner log showed a full answer (2× read_file +
long assistant text, settle OK) but TUI stayed on `thinking…` with only `→ read_file`
rows and status `done`.

## Root cause

1. **Grok session notif buffer was 256 with drop-on-full.** Image turns stream 400+
   `agent_message_chunk` (+ thought) frames; overflow dropped answer chunks while
   earlier tool events still reached the UI.
2. **Race:** `session/prompt` RPC can complete while chunks remain on the notif
   channel; SendTurn returned via `done` without draining, so `lastText` /
   `FinalMessage` could miss the answer.
3. **TUI:** `turnStreamClosedMsg` set `done` without clearing a stranded thinking
   placeholder when deltas never arrived.

## Fix

- Buffer `8192`; on full, **block** instead of drop (until dispatcher dies).
- After `session/prompt` returns, **drain** remaining notifications before
  `emitTerminal`.
- TUI: clear stranded `thinking…` on stream close (system notice if no text).

## Tests

- `grok_message_chunk_drain_test.go` — 400 chunks + delayed RPC reply
- `turn_stream_closed_thinking_test.go` — stream close clears thinking

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Drain Grok message chunks and clear stranded TUI thinking after image turns
# --->8---
