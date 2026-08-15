---
id: CA-520
feature_key: cli-tui
title: /open continue must not replay the old turn (seed SSE cursor)
date: 2026-08-15
status: COMPLETE
---

## Problem

After opening a completed run and sending one more message, the TUI replayed
the **entire previous response** (with the new question appended) before
showing the current turn's real answer. User report + screenshot notes 1-2-3:
"tôi chat tiếp 1 câu thì nó reply lại toàn bộ response của lần trước — append
câu đó của tôi vào — trước khi show câu trả lời thực sự của turn này."

## Root cause (TUI-only, not a runner regression)

`SendTurn` (tui/client/client.go) starts its SSE stream at
`c.lastSeq[runID]` — an internal per-run cursor. The runner's
`/events/stream?afterSeq=N` only emits events with `seq > N`. The runner is
correct; the bug is that the **TUI never seeded the cursor** on resume/open:

- Desktop seeds it: `consumeHistoryReplayStream` reads `handle.lastEventSeq`
  during history-open, and `streamRun`/`sendTurn` update `lastSeq` per event
  (`HttpWsRunnerClient.ts`).
- The TUI model tracks `m.lastEventSeq` (app.go), but `Client.lastSeq` stayed
  at its default `0`. After `/open <completed-run>`, `ChatOpenedMsg` set
  `m.lastEventSeq = handle.LastEventSeq` yet the **client** cursor remained 0.

So the next `SendTurn` opened the SSE at `afterSeq=0` and the runner re-sent
every event of the finished turn. Those replayed `message_delta`s
(`ProviderTurnID` empty for seeded/Grok JSONL history) pass the client's
turnId filter (`ev.ProviderTurnID != "" && ev.ProviderTurnID != turnID`), land
in `appendAssistantDelta`, and append to the trailing assistant bubble. The new
`turn_started` prompt echo then gets concatenated onto the same bubble — exactly
the reported 1-2-3 layout.

## Fix (TUI-only)

**A. `Client.NoteLastSeq(runID, seq)` (client.go)** — monotonic per-run cursor
seed. Never decreases; an older snapshot cannot rewind a live stream. Added a
`sync.Mutex` to `Client` because `NoteLastSeq` is now called from the Bubble Tea
Update loop while `SendTurn`/`StreamRun` goroutines read/write the same map —
previously unsynchronized and now genuinely concurrent on the same run.

**B. Seed the cursor wherever the model advances it:**

- `ChatOpenedMsg` (app.go): `m.client.NoteLastSeq(handle.RunID, handle.LastEventSeq)`.
- `turnStreamEventMsg` / `orchStreamEventMsg`: `NoteLastSeq(runID, ev.Seq)`.
- `openTurnStream` (turn_stream.go): `NoteLastSeq(runID, m.lastEventSeq)`
  before the async `SendTurn` reads the cursor (orchestration events may have
  advanced past the /open snapshot).

A continue turn after `/open` now opens SSE at the resume snapshot instead of 0.

## Provider impact

Provider-agnostic. The cursor is a transport/stream detail on the run id; no
provider adapter or provider-key dispatch reads or writes it. All new tests
parameterize Claude / Codex / Grok (cross-provider-parity Case 1 + parameterized
guard) so a future per-provider branch trips them.

## Tests

New additive files:

`client/client_note_lastseq_test.go`:
- `NoteLastSeq` seeds `SendTurn` to open SSE at `afterSeq=50` (not 0).
- `NoteLastSeq` never decreases (50 then 40 stays 50).
- A second `SendTurn` on the same run resumes from the cursor the first turn's
  events advanced (0 → 3), even without an explicit note.

`app/tui_open_continue_no_replay_test.go`:
- `ChatOpenedMsg` seeds the client cursor from `LastEventSeq=50`; the app's own
  `SendTurn` then requests `afterSeq=50` (claude/codex/grok).
- Full `/open → continue` path against a fake runner that filters events by
  `afterSeq` (real runner contract): old events (seq ≤ 50, empty turn id) are
  **not** re-rendered; only the new turn's deltas become an assistant bubble.
- `/open` with no `LastEventSeq` leaves the cursor at 0 (nothing skipped).

## Contract matrix (durable-replay-contracts)

| Surface | Coverage |
|---|---|
| Durable transcript restored once | `TestOpenContinue_DoesNotReplayOldTurn` — old turn not re-appended |
| Event order preserved | Same test — only seq > cursor renders, in order |
| Interactive state | Unchanged (no card-path edits) |
| Agent lifecycle | Unchanged |
| Terminal state | Unchanged |

Providers exercised: claude, codex, grok (parameterized). No unsupported path.

## Verification

- `go build ./...` clean; `go vet ./internal/tui/... ./internal/cli/...` clean.
- `go test ./internal/tui/... ./internal/cli/... -count=1` all `ok` (legacy
  untouched and green).
- New tests race-clean: `go test ./internal/tui/client -race` and
  `./internal/tui/app -race` on the new tests.

## Out of scope / residual

- Runner SSE `afterSeq` and `startTurn` behavior unchanged.
- `StreamLive` still uses a local `afterSeq` (orch path); the model explicitly
  notes every orch event into the client cursor, so the two stay in sync.
- Pre-existing unrelated suites (`internal/runner` flaky, `internal/structure`
  platform) unchanged.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: The TUI now seeds the per-run SSE cursor (NoteLastSeq) on /open and on every turn/orch event, so a continue turn after opening a completed run streams from the resume snapshot instead of afterSeq=0 replaying the whole old turn into the transcript
# --->8---