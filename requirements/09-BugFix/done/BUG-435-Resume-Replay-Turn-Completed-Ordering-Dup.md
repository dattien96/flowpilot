# BUG-435: Post-restart resume replay emits `turn_completed` before `message_completed`, plus duplicate `turn_completed`

## Metadata

- Document ID: `BUG-435`
- Title: `resumed SSE stream replays turn as turn_started → turn_completed → message_completed → turn_completed`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-46-Test-Steps](../../07-Coding-Plan/done/), [BUG-384](BUG-384-Grok-Retry-Frames-Pollute-Replay.md)
- Feature Keys: `chat-replay`, `durable-replay`

## AI Quick View

### Summary

- After runner restart + `POST /client/workflow-runs/<run>/resume`, the resumed SSE stream replays a single completed turn as 4 events in inverted order: `turn_started` → `turn_completed` → `message_completed` → `turn_completed` (duplicate terminal). Live stream order is `turn_started → message_delta* → turn_completed`. Transcript overlay emits the terminal early and the persisted event log appends a second `turn_completed`.
- Related to BUG-384 (same replay overlay code — `overlayRawTurnPrompts`/`appendTranscriptReplayEventsOpt`) but on the successful single-turn path, not retry pollution.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom**: resume replay shows `turn_completed` before `message_completed`, and the turn's terminal event twice.
- **Expected**: replay ordering matches live ordering; exactly one terminal event per turn.
- **Actual**: transcript-* overlay emits `turn_completed` then `message_completed`; persisted event `evt-46` adds a second `turn_completed` (original `occurredAt`).
- **Impact**: client-visible ordering/duplication on resume; consumers relying on terminal-after-message or unique terminal events misbehave.

## Reproduction

1. Complete a devin chat turn; kill runner; restart.
2. `POST /client/workflow-runs/<run>/resume`; collect SSE.
3. Observe `transcript-*` seq: turn_started → turn_completed → message_completed → turn_completed.

## Root cause

- Replay overlay (`overlayRawTurnPrompts`/`appendTranscriptReplayEventsOpt`, `interactive_resume.go` region) composes transcript-derived events in wrong order and does not dedupe against the persisted event log.

## Evidence

- `~/fp-beds/lt-evidence/cp46/BUG-LIVE-CP46-R3-resume-replay-ordering.md`
- `~/fp-beds/lt-evidence/cp46/r-run38-resume-events.sse` seq 1-4 vs live `r-run38-events.sse` seq 1-6; `run-38-turns.ndjson`.

## Severity

low

## Completion Notes (implemented 2026-09-22, CA-916)

- Root cause: `transcriptAssistantInsertIndex` appended the recovered `message_completed` after the turn's last own event — including after a replayed `turn_completed`, producing `turn_started → turn_completed → message_completed → turn_completed` (dup terminal).
- Fix: the insert index now lands before the turn's first terminal event when one exists.
- Files: `internal/runner/interactive_resume.go`.
- Tests: `bug435_replay_terminal_ordering_test.go`. Provider-agnostic replay path.
