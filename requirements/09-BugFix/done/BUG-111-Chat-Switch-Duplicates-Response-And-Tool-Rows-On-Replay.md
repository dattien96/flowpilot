# BUG-111: Chat Switch Duplicates Response And Tool Rows On Replay

## Metadata

- Document ID: `BUG-111`
- Title: `Chat Switch Duplicates Response And Tool Rows On Replay`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `None`
- Related Documents: [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md), [BUG-109: Child/Main Agent Switch Skips Timeline Events](./BUG-109-Child-Main-Agent-Switch-Skips-Timeline-Events-And-Duplicates-Bus-Messages.md), [BUG-110: History Panel Switch Leaves Thinking Row](./BUG-110-History-Panel-Switch-Leaves-Thinking-Row-After-Replay-Completes.md)
- Replaces: `None`
- Tags: `multi-agent, history-panel, timeline-reducer, idempotency, replay, zustand`

## AI Quick View

### Summary

- Switching between chats in the history panel (and between child/main agent views) re-streams the run from seq 0. On re-delivery the timeline reducer treated already-present events as new, producing a DUPLICATE assistant bubble (the second copy visibly re-streaming, e.g. stuck at "The") and a duplicate tool-group ("1 tool call completed" rendered twice).
- Root cause: `applyTimelineEvent` pushed new timeline items for `message_delta`, `message_completed`, `tool_started`, `tool_completed`, and `turn_started` without checking whether an item produced by the SAME persisted event already existed. Persisted events keep their original ids on replay, so a re-delivery created a second item with a colliding id.
- Fix: make `applyTimelineEvent` idempotent by id/identity — re-delivered events rebuild the existing item in place (or are skipped) instead of duplicating. The normal forward-streaming path is unaffected because freshly emitted events always have ids not yet in the timeline.
- Secondary: `openHistoryRun` did not reset `_runSnapshots`, so a focus/back round-trip after switching chats could restore a stale timeline from an unrelated run. Now reset alongside `_runReplaySeq`.

### Current Ask

- Eliminate duplicate response bubbles and tool rows when switching chats / agent views.

### Key Decisions

- `V-1` Idempotency is enforced in the reducer (single source of truth for timeline construction) rather than per-consumer. This covers ALL re-delivery paths (`consumeHistoryReplayStream`, `consumeAgentStream`, `consumeStream`) at once and is robust to any future stream that re-applies events.
- `V-2` `message_delta` with no active streaming bubble RESUMES an existing bubble with the same id (resetting its text) so a full re-stream rebuilds in place; only a genuinely new id pushes a new bubble.
- `V-3` `message_completed` updates an existing bubble with the same id in place instead of pushing a duplicate.
- `V-4` `tool_started` skips a re-delivered start whose row already exists. `tool_completed` falls back to skipping when no running tool of that name exists but a tool of that name is already present (a re-delivery) — legitimate repeated calls always have a running tool to close, so they are unaffected.
- `V-5` `turn_started` dedupes the prompt by its stable `prompt-${providerTurnId}` id in addition to the existing `hasPendingPrompt` text guard.
- `V-6` `openHistoryRun` resets `_runSnapshots: {}` to prevent cross-run snapshot leakage.

### Constraints

- The fix relies on the server assigning STABLE event ids that survive replay (confirmed: `emitLocked` persists `ev.ID`; `subscribe` replays the persisted `rs.events` verbatim). Forward streaming always uses fresh ids, so the guards never trigger during a live turn.
- No change to seq bookkeeping (`_runReplaySeq`), `backToMainRun` afterSeq (BUG-109), or the orchestration stream split (BUG-110).

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/state/timelineReducer.ts` — `applyTimelineEvent` cases: `turn_started`, `message_delta`, `message_completed`, `tool_started`, `tool_completed`
- `apps/desktop-flowpilot/src/state/store.ts` — `openHistoryRun` (`_runSnapshots: {}` reset)
- `apps/desktop-flowpilot/src/state/timelineReducer.test.ts` — three new idempotency tests

## 1. Issue Summary

After switching between chats in the history panel (or between a child agent and the main agent), the timeline showed the latest response twice. The duplicate copy was actively re-streaming — visibly stuck mid-message (e.g. showing only "The") — and the preceding tool call also appeared twice ("1 tool call completed" rendered as two separate groups). Depending on stream timing the alternative outcome was a lingering "Thinking…" row (addressed separately in BUG-110).

## 2. Parent Links

- coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- task: [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md)
- related bugfixes: [BUG-109](./BUG-109-Child-Main-Agent-Switch-Skips-Timeline-Events-And-Duplicates-Bus-Messages.md), [BUG-110](./BUG-110-History-Panel-Switch-Leaves-Thinking-Row-After-Replay-Completes.md)

## 3. Environment and Reproduction

- environment: Desktop app, any run that contains a streamed assistant response and at least one tool call (e.g. a spawn_agent turn).
- reproduction steps:
  1. Open a chat with a completed/streaming response in the history panel.
  2. Click a different chat in the history list, then click back to the original.
  3. Observe: the last response renders twice (the second copy re-streaming), and the tool call group appears twice.
- frequency: Consistent on chat switch because the run is re-streamed from seq 0 into the rebuilt timeline.

## 4. Expected vs Actual

- expected: Re-opening a chat shows each response and tool call exactly once.
- actual: Re-delivered `message_delta`/`message_completed`/`tool_*` events created duplicate timeline items because the reducer pushed new items without checking for an existing item from the same persisted event.

## 5. Impact

- users affected: All desktop users switching between chats or agent views.
- workflows affected: History panel navigation; child/main agent focus switching.
- severity: High — the transcript visibly duplicates content on a very common interaction.

## 6. Root Cause

- confirmed cause:

  The server assigns a single monotonic per-run seq and persists every event with a stable `ev.ID` (`emitLocked`). `subscribe(runID, after)` replays the persisted `rs.events` verbatim and the SSE handler dedupes only the snapshot/live overlap by seq. So when a chat switch re-streams a run from seq 0, every event arrives again with its ORIGINAL id.

  `applyTimelineEvent` built the timeline by PUSHING new items:
  - `message_delta` (no active stream) pushed a new assistant bubble with `id = e.id`.
  - `message_completed` (no active stream) pushed a new finalized bubble.
  - `tool_started` pushed a new tool row.
  - `tool_completed` (no running tool found) pushed a new tool row.
  - `turn_started` pushed a prompt (guarded only by last-item text match).

  On re-delivery these pushes created a SECOND item with the same id as one already in the timeline — a duplicate bubble / tool group. Because the streaming bubble accumulator (`_streamingAssistantId`) had already been cleared, the re-delivered deltas started a brand-new bubble and re-streamed into it, which is why the duplicate appeared mid-stream.

- evidence:
  - `emitLocked` (`interactive_service.go`) stamps and persists `ev.ID`; `subscribe` returns persisted events unchanged → same ids on replay.
  - New test `"applying a recorded turn twice is idempotent"` reproduces the duplicate before the fix and passes after.

## 7. Fix Strategy

- `F-1` `message_delta`: when there is no active streaming bubble, if a bubble with `e.id` already exists, resume it and reset its text (rebuild in place); otherwise push a new bubble.
- `F-2` `message_completed`: when there is no active streaming bubble, update an existing bubble with `e.id` in place; otherwise push.
- `F-3` `tool_started`: skip if a tool row with `e.id` already exists.
- `F-4` `tool_completed`: after failing to find a running tool of that name, skip if a tool of that name already exists (re-delivery); otherwise push. Legitimate repeated calls always have a running tool to close and never reach this branch.
- `F-5` `turn_started`: dedupe the prompt by its stable `prompt-${providerTurnId}` id in addition to `hasPendingPrompt`.
- `F-6` `openHistoryRun`: reset `_runSnapshots: {}` so a later focus/back round-trip cannot restore a stale timeline from a previously-open run.

## 8. Validation

- `V-1` `tsc -p tsconfig.phase1-tests.json` passes (exit 0).
- `V-2` `timelineReducer.test.js`: 20/20 pass, including three new BUG-111 tests:
  - `"applying a recorded turn twice is idempotent — no duplicate bubbles or tools"`
  - `"re-streaming a finalized bubble (deltas restart) rebuilds it in place"`
  - `"legitimate repeated tool of the same name still records both calls"`
- `V-3` `store.test.js`: 52/52 pass (the BUG-109 `backToMainRun` test was corrected to model the real child→main flow; its prior setup invoked `backToMainRun` while already on the main run, which let `cacheRunSnapshot` overwrite the snapshot cursor and never actually executed before because the suite was previously validated by type-check only).
- `V-4` Manual reproduction in a live multi-agent session could not be run in this environment; the duplicate is reproduced deterministically by the idempotency unit test instead.

## 9. Regression Guard

- tests: Three reducer tests guard idempotency; the corrected store test guards the BUG-109 replay cursor under the real flow.
- audit checks: Any future reducer change that re-introduces an unconditional push for these event types would fail the idempotency test.

## 10. Follow-Up Document Updates

- upstream docs that must change: None — the architectural intent (timeline is reconstructed deterministically from the persisted event log) was implicit; this fix makes the reconstruction idempotent as that intent requires.
- notes left unchanged on purpose: The dual-stream design in `openHistoryRun` (history replay + orchestration) is retained; idempotency makes it robust rather than removing it.
