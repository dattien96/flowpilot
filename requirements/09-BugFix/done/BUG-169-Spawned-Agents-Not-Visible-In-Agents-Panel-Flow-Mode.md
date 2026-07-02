# BUG-169: Spawned Agents Not Visible In Agents Panel (Flow Mode)

## Metadata

- Document ID: `BUG-169`
- Title: `Spawned Agents Not Visible In Agents Panel (Flow Mode)`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-130-131` (agents panel stabilization), `requirements/09-BugFix/done/BUG-132` (merge closed children)
- Replaces: `none`
- Tags: `agent-flow-engine, ui, desktop, orchestration, agents-panel, flow-mode, needs-repro`

## AI Quick View

### Summary

- In a Flow Mode / workflow run (observed on "Review Loop", run-5081), the orchestrator's chat says it launched a coder agent ("Coder agent launched. I'll wait..."), but the Agents panel shows only the `main` card — no coder, and no "Recently closed" entry either.
- Deep tracing (both static and a live experiment against the real server code, see Root Cause) ruled out a server-side registration/attribution defect: a child spawned under a workflow-mode parent registers and lists correctly, same as under a chat-mode parent. No existing test had covered a workflow-mode parent before this bug — that gap is now closed with a regression test.
- What the tracing did surface is a genuine, provable race on the desktop: `refreshAgentRuns`'s HTTP fallback replaced `agentRuns` wholesale, while the live SSE path merges (BUG-132's fix for the reverse direction). `AgentsPanel` fires a `refreshAgentRuns()` the instant `mainRunId` is set — before anything has been spawned — so a slow-to-resolve empty response landing *after* a later SSE update already merged in a freshly-spawned child would wipe that child back out. This fully explains the reported symptom without needing a server-side defect, so it is the applied fix; full live confirmation (with runner logs from the exact moment of the original repro) was not possible in this environment.

### Current Ask

- Every sub-agent the orchestrator spawns during a Flow Mode run must appear in the Agents panel (running, then "Recently closed") for the run the desktop is viewing.
- Done: server-side attribution ruled out via a new regression test; the identified desktop-side race fixed.

### Key Decisions

- `V-1` **(confirmed correct, not the bug)** The parent run id under which children are registered (`registerChild(parentRunID, ...)`) already equals the `mainRunId` the desktop queries via `GET /client/workflow-runs/{runId}/agents`, for both chat-mode and workflow-mode parents — proven by `TestListAgentRunSummariesHTTPWorkflowModeParent`.
- `V-2` **(applied)** `refreshAgentRuns`'s HTTP-fetched result must merge into existing `agentRuns` state, not replace it — the same rule BUG-132 already established for the SSE `agent_graph_updated` path, now applied symmetrically to the HTTP fallback.

### Constraints

- Did not "fix" by broadening the panel's filters — the trace showed the presentation filters (BUG-130/131/132) were never the issue.
- The applied fix does not regress the AI-driven `spawn_agent` (UI + tool) path or the flowRef entry-node path — it only changes how one already-fetched HTTP snapshot is folded into state.

### Open Questions

- If the symptom recurs, capture the runner log line `[agent-spawn] child created parent=%q child=%q ...` (`interactive_service.go:1901`) at the moment of the original repro and compare against the desktop's `GET /client/workflow-runs/{runId}/agents` request — this remains the fastest way to fully confirm vs. rule out a residual cause (e.g. an actual spawn failure narrated as success by the model, which this fix cannot address since that is a model-behavior issue, not a code defect).

### Source Refs

- `apps/desktop-flowpilot/src/components/AgentsPanel.tsx` (`orderedRuns`, `activeRuns`, `closedRuns` — render from `agentRuns`)
- `apps/desktop-flowpilot/src/state/store.ts:467` (`refreshAgentRuns` → `client.listAgentRuns(parentRunId)`, `parentRunId = mainRunId ?? runId`)
- `apps/desktop-flowpilot/src/state/store.ts:2035` / `:2110` (`agent_graph_updated` merged into `agentRuns`)
- `apps/local-runner/internal/runner/interactive_service.go:1905` (`registerChild(parentRunID, handle.RunID)`)
- `apps/local-runner/internal/runner/interactive_service.go:2001` (`listAgentRunSummaries(parentRunID)`)
- `apps/local-runner/internal/runner/interactive_service.go:1018` (`emitAgentGraphLocked` — emits only if `s.runs[parentRunID] != nil`, on that run's stream)
- `apps/local-runner/internal/runner/interactive_service.go:1901` (`[agent-spawn] child created parent=%q child=%q ...` — the confirming log line)

## 1. Issue Summary

During a Flow Mode workflow run, the orchestrator (`main`, codex) reports launching a coder sub-agent, but that sub-agent never shows in the Agents panel. The `main` card is present and idle; no child card and no "Recently closed" section appear, indicating the desktop's `agentRuns` list is empty for the viewed run.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop-flowpilot on branch `task/gemini-adapter`; a runner executing a "Review Loop" workflow run (`workflow_step_auto`). Observed run label `run-5081`, status Completed.
- reproduction steps:
  1. Launch the "Review Loop" workflow with a task prompt.
  2. Let the orchestrator run; it states it is spawning a coder agent (and later reviewers).
  3. Observe the Agents panel shows only `main` — no coder card appears, live or after completion.
- frequency: unknown / to be confirmed (only one screenshot-confirmed occurrence).

## 4. Expected vs Actual

- expected: the coder sub-agent appears in the Agents panel as it is spawned (Running), then moves to "Recently closed" when it finishes; likewise the reviewers.
- actual: only the `main` orchestrator card is shown; spawned children are absent.

## 5. Impact

- users affected: anyone using Flow Mode with agent orchestration (Review Loop and similar multi-agent flows).
- workflows affected: multi-agent orchestration visibility; the run may still function, but the user cannot see, focus, or inspect the sub-agents.
- severity: high — the Agents panel is the primary surface for multi-agent orchestration; if children are invisible the feature appears broken.

## 6. Root Cause

Investigated three hypotheses:

- (A) **Parent attribution mismatch** — traced `spawn_agent` end-to-end for both the Claude MCP path (`handleClaudeSpawnAgent` → `bridge.SpawnAgent` → `svc.spawnChildRun(ctx, b.rs.id, in)`) and the Codex path (`codex_adapter.go` dynamic-tool dispatch, identical `bridge.SpawnAgent` call) — both use the run whose turn is currently executing as `parentRunID`, i.e. exactly the hub/main run. Then built `TestListAgentRunSummariesHTTPWorkflowModeParent` (new, in `agent_orchestrator_test.go`) — the workflow-mode analogue of the existing `TestListAgentRunSummariesHTTP`, which only ever covered a `normal_chat` parent. **Ruled out**: a workflow-mode parent (`runKind: "workflow"`) spawns and lists a child exactly like a chat-mode parent does. `emitAgentGraphLocked` also emits onto the same run's own event stream (`s.emitLocked(rs, ev)`), which is the same channel `sendTurn`'s live stream reads from — no separate/wrong channel involved.
- (B) **Spawn actually failed, narrated as success** — plausible (a model can misreport a failed tool result), but not fixable at the code level: `handleClaudeSpawnAgent`/the Codex equivalent already return the real error text as the tool result, so the model has the information; if it still narrates success, that is model behavior, not a code defect. Left as an open, un-confirmable-without-logs possibility.
- (C) **Frontend TOCTOU race — confirmed as a real, provable defect (though not proven to be *the* live occurrence, since no runner logs from the original repro were captured):** `AgentsPanel`'s `useEffect(() => { void refreshAgentRuns(); }, [refreshAgentRuns, mainRunId])` (`AgentsPanel.tsx:67-69`) fires the instant `mainRunId` is set — i.e. before the user has sent a prompt or anything has spawned — issuing an HTTP `GET /client/workflow-runs/{runId}/agents` that will correctly return empty at that moment. `refreshAgentRuns` (`store.ts:467-487`) applied its result with `set({ agentRuns })` — a wholesale **replace**, unlike the SSE `agent_graph_updated` path which explicitly **merges** (`mergeAgentRunsById`, established by BUG-132 specifically because "replacing wholesale dropped ... entries"). The `_agentRunsLoadSeq` guard only protects against a newer HTTP request being superseded by an even newer one — it does not protect against an HTTP *response* arriving after a *live SSE update* already merged a freshly-spawned child into state. If that empty response resolves slowly enough to land after the SSE-driven merge, it wipes the just-spawned child straight back out of `agentRuns`, and nothing else repopulates it once the child has already finished running.
- evidence: `AgentsPanel.tsx:67-69` (mount-time refresh trigger); pre-fix `store.ts:481` (`set({ agentRuns })`); `store.ts:2018-2027` (`mergeAgentRunsById`, BUG-132's own rationale for why replace-on-arrival is wrong for this exact kind of racing update).

## 7. Fix Strategy

- `F-1` **(implemented)** `refreshAgentRuns` now merges its HTTP result into existing state via `mergeAgentRunsById`, matching the SSE path, instead of replacing wholesale (`store.ts`).
- `F-2` **(implemented)** Added `TestListAgentRunSummariesHTTPWorkflowModeParent` (`agent_orchestrator_test.go`), closing the workflow-mode-parent test gap and formally ruling out hypothesis (A) as a regression guard against it reappearing.
- Hypothesis (B) has no code-level fix; hypothesis (C) is addressed by `F-1`.

## 8. Validation

- `V-1` `TestListAgentRunSummariesHTTPWorkflowModeParent` — passes (confirms (A) is not the cause).
- `V-2` Full runner `go test ./...` — no new failures from the `agent_orchestrator_test.go` addition (two pre-existing, unrelated failures confirmed present without any of this session's changes: `TestSkillsMerge*WithPrecedence`, `TestStartInteractiveAuthLaunchesFromWorkspace`).
- `V-3` `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- `V-4` Not executed: a live repro of the original "Review Loop" scenario to confirm the race in `F-1` is the actual mechanism behind the reported symptom (vs. hypothesis (B), which would need the same live capture to rule in/out). The user should watch for a recurrence after this fix; if it recurs, capture the diagnostic noted in Open Questions.

## 9. Regression Guard

- tests: `TestListAgentRunSummariesHTTPWorkflowModeParent` (new) guards hypothesis (A); existing `TestListAgentRunSummariesHTTP`, BUG-130/131/132 panel tests unaffected.
- alerts: none.
- audit checks: recorded in `change-audit/CA-206-agents-panel-refresh-merge-race.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: hypothesis (B) (a genuinely failed spawn narrated as a success by the model) is intentionally left unresolved — it is a model-behavior question, not a code defect this fix can address, and would need a live repro with runner logs to confirm or rule out.
