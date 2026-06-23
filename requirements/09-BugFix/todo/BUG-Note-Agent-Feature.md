# BUG-Note-Agent-Feature: Agent Feature Bug Notes

## Metadata

- Document ID: `BUG-Note-Agent-Feature`
- Title: `Agent Feature Focus Replay And History State Regression`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-20`
- Last Updated: `2026-06-21`
- Parent Documents: [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md), [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/todo/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md)
- Child Documents: `None`
- Related Documents: [Task-082: Spawn-Agent Tool And Orchestrator Core](../../08-Task/done/Task-082-Spawn-Agent-Tool-And-Orchestrator-Core.md)
- Replaces: `None`
- Tags: `agent-feature, desktop, spawn-agent, focus-replay, history, regression`

## AI Quick View

### Summary

- SD-16 Test 2 exposed a desktop replay/focus regression after a completed `spawn_agent` run.
- Completed main runs could show `Thinking...` forever when history replay emitted a nonterminal event and then stayed open.
- Child focus could show main-run assistant text because replay consumers trusted the attached stream instead of checking each event's owning run.
- Repeated child/main switching could leave stale parent replay streams open because `backToMainRun` did not attach an abort controller.
- Follow-up: mock/dev history could still expose child agent rows after main/sub-agent switching because mock history did not mirror the runner's child-run exclusion contract.
- Follow-up: live runner history could expose legacy/orphan sub-agent rows when persisted child sessions lacked `parentRunId`/agent metadata.
- Follow-up: child focus could still render a main response or corrupt the latest main assistant bubble when a stale `_streamingAssistantId` survived the main/sub-agent view switch.
- Follow-up: live in-memory history could still expose an orphan child row during main/sub-agent switching when the row lost `parentRunId` but retained a built-in sub-agent prompt shape.
- Follow-up correction: treating generic `agentName`/`agentStatus` as child evidence hid valid main/root rows from History.
- Follow-up correction: the persisted-session restart path also treated generic agent metadata as child evidence, so normal chats could disappear from History after server restart.
- Follow-up split: SD-16 Test 4 priority-tier failure is captured separately in [BUG-104](./BUG-104-Desktop-Codex-Child-Spawn-Failed-On-Unsupported-Priority-Tier.md).

### Current Ask

- Capture the root cause and solution for SD-16 section 14 Test 2, then preserve regression coverage.

### Key Decisions

- `V-1` Transcript events must only mutate the active timeline when `event.workflowRunId` matches the focused/replayed run.
- `V-2` Parent orchestration graph/bus events match by payload parent run ID, not by child transcript ownership.
- `V-3` Terminal run replay must not leave a synthetic `Thinking...` item visible if the replay stream is incomplete or open-ended.
- `V-4` Back-to-main replay must be abortable before another child focus begins.
- `V-5` Main History must list parent/root runs only; child agent runs belong in the Agents panel, not the History panel.

### Constraints

- Keep the fix inside desktop state/replay handling.
- Do not change provider spawn semantics or `spawn_agent` runtime contracts.
- Preserve raw user observations in this note for later split-out bugs.

### Open Questions

- Should the broader raw notes for missing child sidebar rows and YOLO approval visibility be split into separate BugFix files?

### Source Refs

- `SD-16` Section 11 validation strategy and Section 13 QA.
- Raw SD-16 Test 2 notes below.
- Code: `apps/desktop-flowpilot/src/state/store.ts`, `apps/desktop-flowpilot/src/state/store.test.ts`, `apps/local-runner/internal/runner/interactive_handlers.go`.

## 1. Issue Summary

SD-16 Test 2 failed after a completed `spawn_agent` chat was reopened and the user switched between the main agent and child agent views.

Observed failures:

- completed main history replay showed `Thinking...` forever
- child agent view showed main-agent response text above the child breadcrumb/title
- returning to main appended an incomplete duplicate response
- repeated child/main/history switching eventually made chat switching appear hung
- follow-up: after switching between main chat and sub-agent, child agent chat could still appear in the History panel.
- follow-up evidence: the live runner returned `run-31` in `/client/projects/{projectId}/workflow-runs` with a reviewer sub-agent prompt, `CHILD_AGENT_DONE`, and no `parentRunId`/agent fields, so the History panel treated it as a root chat.
- follow-up: later screenshots still showed `CHILD_AGENT_DONE` from the main transcript above the child header and an incorrect latest assistant row in the main transcript.
- follow-up: after additional main/sub-agent switching, sub-agent chat could still appear in the main History panel while the runner was live.

## 2. Parent Links

- impacted coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- impacted tech design: [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- impacted system spec: [SS-06: Workflow Skill Agent](../../05-System-Specs/SS-06-Workflow-Skill-Agent.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: desktop FlowPilot agent feature, completed `spawn_agent` chat, SD-16 Test 2
- reproduction steps:
  1. Run the SD-16 Test 2 spawn-agent flow until the main run completes.
  2. Open the completed run from history.
  3. Focus the child/sub-agent view.
  4. Press `Back to main agent`.
  5. Repeat child/main switching and then switch history chats.
  6. Observe whether the child row appears in the main History panel.
- frequency: reproducible from the user note.

## 4. Expected vs Actual

- expected: completed history opens without `Thinking...`; child focus shows only child transcript; back-to-main restores only main transcript; repeated switching remains responsive.
- actual: main history could keep `Thinking...`; child view displayed main response text; back-to-main appended a partial duplicate response; repeated switching could leave stale replay streams alive.
- follow-up actual: main History could include an orphaned reviewer child row and open it directly as `main > reviewer`.
- follow-up actual: sub-agent focus could inherit a parent assistant accumulator, so child replay either appended into the wrong assistant bubble or rendered with stale parent content around the child transcript.

## 5. Impact

- users affected: desktop users testing or using multi-agent spawn runs.
- workflows affected: agent focus navigation, history replay, child/main transcript switching.
- severity: high for the agent feature because it corrupts visible transcripts and can make navigation appear hung.

## 6. Root Cause

- hypothesis: focus/history replay streams were applying stale or wrong-run events into whichever transcript was currently visible.
- confirmed cause:
  - `consumeHistoryReplayStream`, `consumeAgentStream`, and related consumers checked the requested stream run, but did not verify each event's `workflowRunId` before applying it to the active timeline.
  - terminal replay for completed runs could apply a `turn_started` event and leave the synthetic `Thinking...` item visible if the SSE stream stayed open without a terminal event.
  - `backToMainRun` started a parent replay stream without an abort controller, so repeated focus/back cycles could leave stale fetches alive until another event arrived.
  - follow-up: `MockRunnerClient.listRunHistory` did not mirror the real runner's history behavior. The real runner excludes runs with `parentRunId`, while the mock history source did not filter child runs and did not preserve child metadata in the history DTO.
  - follow-up: real persisted history could still leak older child sessions when the stored row had no `ParentRunID`, `AgentName`, `Role`, or `AgentStatus`. The project history endpoint only filtered known child metadata, so a legacy reviewer prompt looked like a standalone root chat.
  - follow-up: `focusAgentRun` used an empty fallback timeline for a child without a cached snapshot, but did not clear the previous run's `_streamingAssistantId` or other transient per-run fields. `applyTimelineEvent` also kept a previous assistant accumulator open across `turn_started`, so a new turn could write into the wrong assistant item.
  - follow-up: `projectRunHistory` filtered live in-memory rows only by `parentRunID`, while persisted rows used stricter child-run detection. A live orphaned child row with a built-in sub-agent prompt shape but no `parentRunID` could therefore still enter main History until restart/persistence filtering removed it.
  - follow-up correction: a generic `agentName`/`agentStatus` filter was too broad because valid main/root rows can also carry agent-like metadata while live.
  - follow-up correction: the persisted-session history filter kept using generic `AgentName`/`Role`/`AgentStatus` as child evidence, so those main/root rows could reappear live but disappear after runner restart.
- evidence:
  - Added regression tests reproduce incomplete terminal replay, wrong-run child focus replay, and un-aborted back-to-main replay.
  - The tests pass after filtering events by run ownership and making main replay abortable.
  - Live API inspection matched the screenshot: `run-31` had a reviewer sub-agent prompt and `CHILD_AGENT_DONE` but no parent/agent metadata.

## 7. Fix Strategy

- `F-1` Filter transcript events by `event.workflowRunId === activeRunId` before applying them to a focused/history timeline.
- `F-2` Allow `agent_graph_updated` and `agent_bus_message` through when their payload `parentRunId` matches the parent run.
- `F-3` During terminal replay, restore the known terminal status and remove synthetic `Thinking...` after nonterminal replay events.
- `F-4` Use the resumed child run status when focusing a child without a cached snapshot.
- `F-5` Attach an abort controller to `backToMainRun` replay and abort it before another focus stream starts.
- `F-6` Make mock-spawned children inherit parent project/workflow metadata and exclude child runs from `MockRunnerClient.listRunHistory`, matching the local runner contract.
- `F-7` Exclude persisted history rows that have child metadata or match the built-in coder/reviewer/tester sub-agent prompt prefix, covering legacy orphan rows without hiding active root workflow runs.
- `F-8` Restore child focus through a full clean run snapshot when no cached child snapshot exists, clearing `_streamingAssistantId`, approvals/questions, token usage, and active step state.
- `F-9` Close any previous assistant accumulator on `turn_started` so a new turn always starts from a clean assistant bubble.
- `F-10` Exclude live in-memory history rows only when they have strong child evidence: `parentRunID` or a built-in sub-agent prompt prefix.
- `F-11` Hide desktop History rows only when they have `parentRunId` or a built-in sub-agent prompt prefix; keep main/root rows visible even if they carry agent-like metadata.
- `F-12` Apply the same strong-child-evidence rule to persisted-session history after restart.

## 8. Validation

- `V-1` `npm --prefix apps/desktop-flowpilot run typecheck` passed.
- `V-2` `apps/desktop-flowpilot/node_modules/.bin/tsc -p tsconfig.phase1-tests.json` passed.
- `V-3` compiled store regression suite passed: `46` tests, `0` failures.
- `V-4` `npm --prefix apps/desktop-flowpilot run test:phase1` could not run as-is in this environment because Node 25 treated the directory target `.phase1-tests/tests/phase1` as a missing module; running with an explicit glob exposed unrelated harness dependency/path issues.
- `V-5` broader phase harness with `NODE_PATH=apps/desktop-flowpilot/node_modules` still has one unrelated import-boundary path issue: it scans `/Users/tiendat/Desktop/packages/flowpilot-client-core/src/domain`.
- `V-6` `go test ./internal/runner -run 'ProjectRunHistory' -count=1 -v` passed, covering project filtering, live child exclusion, persisted child exclusion, and legacy orphan prompt exclusion.
- `V-7` `npm --prefix apps/desktop-flowpilot run typecheck` passed after the live orphan metadata history fix.
- `V-8` `apps/desktop-flowpilot/node_modules/.bin/tsc -p tsconfig.phase1-tests.json` passed after the live orphan metadata history fix.
- `V-9` `node --test .phase1-tests/apps/desktop-flowpilot/src/components/navigatorHistory.test.js` passed, covering Navigator filtering for built-in orphan child prompts and preservation of main rows with agent-like metadata.
- `V-10` `go test ./internal/runner -run 'ProjectRunHistory' -count=1 -v` passed after adding persisted restart coverage for main rows with agent-like metadata.

## 9. Regression Guard

- tests:
  - `openHistoryRun does not leave Thinking visible for a completed history replay without a terminal event`
  - `focusAgentRun ignores replay events that belong to the main run`
  - `backToMainRun aborts its replay stream before focusing a child again`
  - `focusAgentRun clears stale parent assistant accumulator before child replay`
  - `applyTimelineEvent starts a new assistant bubble for a new turn`
  - `MockRunnerClient keeps child agent runs out of main history`
  - `TestProjectRunHistoryExcludesLegacyOrphanAgentPromptRuns`
  - `TestProjectRunHistoryExcludesLiveOrphanAgentMetadataRuns`
  - `TestProjectRunHistoryKeepsLiveRootRowsWithAgentMetadata`
  - `TestProjectRunHistoryKeepsPersistedRootRowsWithAgentMetadataAfterRestart`
  - `filterVisibleHistory removes orphan agent rows with built-in child prompts`
  - `filterVisibleHistory keeps main rows with agent-like metadata`
- alerts: none added.
- audit checks: GitNexus impact was LOW for `focusAgentRun`, `backToMainRun`, `consumeAgentStream`, `consumeHistoryReplayStream`, and `consumeStream`. `applyEvent` was CRITICAL, so the fix avoided changing it.

## 10. Follow-Up Document Updates

- upstream docs that must change: no SD-16 contract change required; behavior now matches SD-16 Sections 7, 8, and 11.
- notes left unchanged on purpose: raw notes below still include separate potential bugs for child sidebar creation, YOLO approval visibility, and chat persistence after restart.

## 11. Raw Intake Notes

1. History show max 5 items. Must do collapsible like Remote Chats Does
2. Regression, after sent prompt, the
   prompt was not del from chat box
3. Agent feature: when i said
   (Use spawn_agent exactly once with agent="reviewer",
   provider="codex", wait=true.
   Child prompt: "Do not use tools. Reply exactly: CHILD_AGENT_DONE."
   After the child returns, tell me the exact child result.)
   -> no new agent views created, both in sidebar and in
   item of main chat
   -> then i got

- I’m using a single reviewer sub-agent exactly as requested, with no additional
  tools in the child. After it returns, I’ll report the exact result string verbatim. -> CHILD_AGENT_DONE.

4. Keep YOLO = Off
   I sent (Use spawn_agent exactly once with agent="coder", provider="codex", wait=true.
   Child prompt: "Create a file called child-agent-yolo-off.txt in the current directory with the text 'child approval works'. Then reply exactly: CHILD_APPROVAL_DONE."
   Do not create the file yourself. Only the child agent should do it.)

-> Result is

- Hanging in main
- no new sub-agent view show
- but i see the file child-agent-yolo-off.txt was created

-> Expect

- Main not hang
- Sub agent show and wait approve for me cause YOLO = off
- The approve icon sate must show in main agent and the side bar sub-agent too
- after approve -> sub agent create file -> response to main -> main end

5. Restart sever

- Only chat of item 3 showed -> Chat of item 4 was not show, that mean the chat actually was not started?

6. 2026-06-20 regression: opening old spawn-agent chat hangs app

- Repro: finish a `spawn_agent` chat, then click the old chat from history without restarting the server.
- Actual: the selected chat cannot be accessed and the whole app appears blocked.
- Expected: chat switch returns immediately, then timeline/agent graph replay can continue in the background.
- Root cause: desktop `openHistoryRun` awaited `consumeStream(client.streamRun(runId, 0))`. `streamRun` is an SSE stream and may stay open forever for resumed/history runs, so the history-click action never resolved.
- Fix: `openHistoryRun` now starts history replay as a background task, starts the orchestration graph stream separately, and only uses a bounded history replay helper for terminal/gated statuses.
- Regression guard: added `openHistoryRun returns after attaching an open-ended history stream` in `apps/desktop-flowpilot/src/state/store.test.ts`.

# Test result for SD-16

## Test 2

- After done
- Click to see Sub-agent
  -> click main agent in history tab agent
  -> can see main but the text Thinking... showed forever even the flow is done

-> press to see child agent
Now 1 response form main
(The exact child result returned to me was:

```json
{
  "runId": "run-31",
  "providerSessionId": "thread-32",
  "providerKey": "codex",
  "status": "completed"
}
```

No sub-agent text content was included in the returned result.)

Show at top of child agent, above the title Baack to main agent

- I press Back to main agent
  Main agents show with the latest message is
- The right response
- Append to that is 1 incorrect respose: The ..... without full data
  I think this one is replay again the latest respose is (The exact child result returned to me was:) above. Then it is not correct. Do not replay here

- I press sub-agent againt

* the issue res from main show ontop shill there as above
* Now press go to main agent by item in right side bar -> same behoviou error like press Back to main agent above

- Try to swtich chat in menu history

* the main agent always show Thinking...
* Test to open child - back some time
  Then got REGRESSION BUG. The app hang. i can not swithc chat again
