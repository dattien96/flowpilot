# BUG-116: Child Agent Transcript And Panel Show Duplicate Entries

## Metadata

- Document ID: `BUG-116`
- Title: `Child Agent Transcript And Panel Show Duplicate Entries`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `None`
- Related Documents: [BUG-111: Chat Switch Duplicates Response And Tool Rows On Replay](./BUG-111-Chat-Switch-Duplicates-Response-And-Tool-Rows-On-Replay.md), [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md)
- Replaces: `None`
- Tags: `multi-agent, codex, timeline-reducer, agents-panel, duplicate, agent-graph`

## AI Quick View

### Summary

- A focused child agent's transcript showed its single reply (`CHILD_AGENT_DONE`) repeated three times, with no intervening prompt — i.e. one turn rendered as three identical bubbles.
- Root cause: the Codex event mapper derives a `message_completed` from more than one protocol event for the same logical assistant message (`agent_message`/`turn.message` AND `item/completed`→`agentMessage`). Each carries a distinct runner event id, so the BUG-111 id-based dedup did not collapse them and the reducer stacked identical bubbles.
- Fix: a duplicate-emission guard in `applyTimelineEvent` — a `message_completed` whose text equals the immediately-preceding finalized assistant bubble (no streaming in progress) is treated as a re-emission and skipped.
- Also fixed a related latent double-count: `AgentOrchestrator.graphSnapshot` concatenated historical + live children without dedup (unlike `listAgentRunSummaries`), so a child present in both could appear twice in the Agents panel.

### Current Ask

- Stop the focused child transcript from showing the same reply multiple times.
- Stop the Agents panel from double-listing a child run.

### Key Decisions

- `V-1` Reducer guard is text-scoped and only fires when the LAST timeline item is an identical finalized assistant bubble — so genuinely different consecutive messages still render. (Two identical consecutive assistant messages are virtually always a duplicate emission, not intent.)
- `V-2` `graphSnapshot` now dedups by runId, preferring the live summary (current status) over historical — matching the dedup `listAgentRunSummaries` already performs.
- `V-3` The number of distinct child runs shown in the panel is NOT changed by this fix when they are genuinely distinct runs; multiple real `spawn_agent` calls by the orchestrator remain visible (that is orchestrator behavior, traceable via the `[agent-spawn] request` log lines).

### Constraints

- The transcript guard does not touch the `streamingAssistantId` live path (a streamed message finalizes in place); it only guards the no-stream push path where the extra emissions arrive.
- No change to seq/replay bookkeeping (BUG-109/112) or the orchestration-vs-timeline split (BUG-110).

### Open Questions

- Whether the Codex mapper should suppress the duplicate `message_completed` at the source is left as a possible follow-up; the reducer guard is provider-agnostic and fixes the user-visible symptom for all providers.

### Source Refs

- `apps/desktop-flowpilot/src/state/timelineReducer.ts` — `message_completed` duplicate-emission guard
- `apps/local-runner/internal/runner/agent_orchestrator.go` — `graphSnapshot` dedup by runId
- `apps/local-runner/internal/runner/codex_event_mapper.go` — the multiple `EventMessageCompleted` sources

## 1. Issue Summary

When focusing a child agent, the transcript rendered the child's single reply multiple times (observed: `CHILD_AGENT_DONE` ×3). Separately, the Agents panel could list the same child run more than once.

## 2. Parent Links

- coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- task: [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md)
- related bugfix: [BUG-111](./BUG-111-Chat-Switch-Duplicates-Response-And-Tool-Rows-On-Replay.md)

## 3. Environment and Reproduction

- environment: Desktop + local runner, a Codex child agent.
- reproduction steps:
  1. Spawn a Codex sub-agent that replies with a short fixed message.
  2. Open the child in the Agents panel.
  3. Observe the reply rendered multiple times in one turn.
- frequency: Whenever the Codex stream surfaces the final message via more than one event kind.

## 4. Expected vs Actual

- expected: one assistant bubble per logical message; one panel row per child run.
- actual: one logical message rendered as several identical bubbles; a child could appear twice in the panel via the graph snapshot.

## 5. Impact

- users affected: multi-agent users viewing child transcripts; the Agents panel.
- severity: Medium — confusing/incorrect transcript and panel, no data loss.

## 6. Root Cause

- confirmed cause (transcript): `codex_event_mapper.go` maps several Codex events to `EventMessageCompleted` for the same assistant message (`agent_message`/`turn.message` at the streaming layer and `item/completed`→`agentMessage`). `emitLocked` stamps each with a unique id, so the BUG-111 id dedup does not match them, and `applyTimelineEvent` pushes one bubble per emission.
- confirmed cause (panel): `graphSnapshot` returned `historical[parent] ++ live[parent]` with no dedup; `listAgentRunSummaries` already deduped, so the two panel-update paths disagreed and the graph path could double-list a child.
- evidence: screenshot shows three identical `CHILD_AGENT_DONE` bubbles in one turn (no intervening prompt) and four `reviewer` rows in the panel. New reducer test reproduces the ×3 collapse.

## 7. Fix Strategy

- `F-1` In `applyTimelineEvent` `message_completed` (no-streaming push path), skip the push when the last timeline item is a finalized assistant bubble with identical text.
- `F-2` In `graphSnapshot`, dedup runs by runId (live preferred over historical).

## 8. Validation

- `V-1` `tsc -p tsconfig.phase1-tests.json` passes.
- `V-2` `timelineReducer.test.js`: 22/22, including:
  - `"duplicate message_completed emissions for one message collapse to one bubble (BUG-116)"`
  - `"two genuinely different consecutive messages both render (BUG-116 guard is text-scoped)"`
- `V-3` Go `build`/`vet` clean; orchestrator/agent tests pass (19).
- `V-4` Live verification deferred to the user's next multi-agent run; the symptom is reproduced deterministically by the unit tests.

## 9. Regression Guard

- tests: the two new reducer tests guard the collapse and the text-scoping; orchestrator tests cover the graph path.
- note: distinct real child runs still render distinctly — the guard never merges different messages or different runs.

## 10. Follow-Up Document Updates

- upstream docs that must change: None.
- possible follow-up: suppress the duplicate `message_completed` in `codex_event_mapper.go` at the source; the panel "N reviewers" count, when the runs are genuinely distinct, reflects how many times the orchestrator called `spawn_agent` (see `[agent-spawn] request` in the runner log).
