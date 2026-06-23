# BUG-112: History Reopen Truncates Multi-Turn Run To First Turn

## Metadata

- Document ID: `BUG-112`
- Title: `History Reopen Truncates Multi-Turn Run To First Turn`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `None`
- Related Documents: [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md), [BUG-111: Chat Switch Duplicates Response And Tool Rows On Replay](./BUG-111-Chat-Switch-Duplicates-Response-And-Tool-Rows-On-Replay.md)
- Replaces: `None`
- Tags: `history-panel, replay, multi-turn, runner, run-handle, zustand`

## AI Quick View

### Summary

- After sending a second prompt in a chat (turn 2), switching to another chat and back showed the PREVIOUS response (turn 1) as the latest — the most recent turn was missing.
- Root cause: `openHistoryRun` replays the run from seq 0 over an SSE stream that stays open. `consumeHistoryReplayStream` stopped at the FIRST event matching the resumed status (for `completed`, the first `turn_completed`). A multi-turn run has several `turn_completed` events, so replay halted after turn 1.
- Fix: the runner now reports `lastEventSeq` (the seq of the last persisted event) on the resume handle. Replay stops only when it reaches that seq, so the full multi-turn transcript is rebuilt. A still-`running` run keeps live-tailing. Runners that predate the field fall back to the old first-terminal-event stop.

### Current Ask

- Re-opening a multi-turn chat from the history panel must show the latest response, not the previous one.

### Key Decisions

- `V-1` Surface `RunHandle.lastEventSeq = seq of last persisted event` from the runner (`resumeRun`), using `rs.events[len-1].Seq` so it always equals the last event the client will receive in the replay snapshot.
- `V-2` `shouldStopHistoryReplay` stops on `e.seq >= lastEventSeq` for terminal/waiting statuses; `running`/`starting` never stop on the cursor (more events arrive live).
- `V-3` Backward compatible: when `lastEventSeq` is absent (older runner), fall back to the previous first-terminal-event stop condition.

### Constraints

- Requires the local runner to be rebuilt for the fix to take effect; the desktop change degrades gracefully (old behavior) against an un-rebuilt runner.
- No change to the live-turn path (`sendPrompt`/`consumeStream`) or the orchestration stream.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/provider_event.go` — `RunHandle.LastEventSeq`
- `apps/local-runner/internal/runner/interactive_handlers.go` — `resumeRun` populates `LastEventSeq`
- `apps/desktop-flowpilot/src/types/contract.ts` — `RunHandle.lastEventSeq`
- `apps/desktop-flowpilot/src/state/store.ts` — `openHistoryRun`, `consumeHistoryReplayStream`, `shouldStopHistoryReplay`

## 1. Issue Summary

In a chat with more than one turn, re-opening the chat from the history panel rendered the transcript only up to the first turn's response. The latest turn's response was missing, so the displayed "latest" answer was actually the previous one.

## 2. Parent Links

- coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- task: [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md)
- related bugfix: [BUG-111](./BUG-111-Chat-Switch-Duplicates-Response-And-Tool-Rows-On-Replay.md)

## 3. Environment and Reproduction

- environment: Desktop app + local runner, any chat with ≥2 turns.
- reproduction steps:
  1. Open a chat and let a response complete (turn 1).
  2. Send another prompt and let it complete (turn 2).
  3. Click a different chat in the history panel, then click back.
  4. Observe: the transcript ends at turn 1's response; turn 2 is missing.
- frequency: Consistent for completed multi-turn runs.

## 4. Expected vs Actual

- expected: Re-opening replays every turn; the latest response is shown.
- actual: Replay stopped at the first `turn_completed`, truncating the transcript to turn 1.

## 5. Impact

- users affected: All desktop users re-opening chats that have more than one turn.
- workflows affected: History panel navigation; any iterative chat.
- severity: High — the user sees stale/incorrect "latest" content after a very common navigation.

## 6. Root Cause

- confirmed cause:
  `openHistoryRun` starts `consumeHistoryReplayStream` on `streamRun(runId, 0)`. The SSE endpoint replays all persisted events then holds the connection open for live events, so the replay consumer must stop itself. `shouldStopHistoryReplay` stopped at the first event matching the resumed status:
  ```ts
  if (resumedStatus === "completed") return e.type === "turn_completed";
  ```
  A multi-turn run emits one `turn_completed` per turn, so the loop `break`ed after turn 1 and never applied turns 2..N.
- evidence:
  - Code trace in `shouldStopHistoryReplay`.
  - New regression test `"openHistoryRun replays all turns of a completed multi-turn run"` fails with the old stop condition and passes with the seq cursor.

## 7. Fix Strategy

- `F-1` Runner: add `LastEventSeq int64` to `RunHandle`; in `resumeRun` set it to `rs.events[len-1].Seq` (the last persisted event the client will receive).
- `F-2` Desktop: add `lastEventSeq?: number` to `RunHandle`; thread `handle.lastEventSeq` into `consumeHistoryReplayStream`.
- `F-3` Desktop: `shouldStopHistoryReplay` stops on `e.seq >= lastEventSeq` for terminal/waiting statuses, never for `running`/`starting`; fall back to the old first-terminal-event stop when `lastEventSeq` is absent.

## 8. Validation

- `V-1` `go build ./internal/runner/` and `go vet` pass.
- `V-2` `tsc -p tsconfig.phase1-tests.json` passes (exit 0).
- `V-3` `store.test.js`: 53/53 pass, including the new `"openHistoryRun replays all turns of a completed multi-turn run (BUG-112)"`.
- `V-4` `timelineReducer.test.js`: 20/20 pass.
- `V-5` The 7 failing Go tests (`codex_resume_process_test.go`, `cross_account_resume_test.go`) are pre-existing — they exec the real `codex` CLI binary which is unavailable in this environment; confirmed by stashing the change and re-running (same failure).
- `V-6` Manual verification in the live app could not be run here; reproduced deterministically by the unit test.

## 9. Regression Guard

- tests: New store test asserts both turns of a completed run are replayed.
- audit checks: Any reversion of `shouldStopHistoryReplay` to a first-terminal-event stop would fail the multi-turn test.

## 10. Follow-Up Document Updates

- upstream docs that must change: None — replay was intended to reconstruct the full transcript; this enforces that for multi-turn runs.
- notes left unchanged on purpose: The fallback path preserves behavior for runners without `lastEventSeq`.
