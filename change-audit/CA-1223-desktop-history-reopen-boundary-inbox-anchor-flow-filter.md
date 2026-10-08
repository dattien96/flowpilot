# CA-1223 — desktop: orchestration stream starts at durable boundary on history reopen; inbox popover anchored to header; internal sprint flows out of workflow select; settle_pending out of inbox; worktree toggle in workflow mode

- **Area**: apps/desktop-flowpilot (state/store.ts, state/workingMode.ts,
  state/attentionQueue.ts, components/ChatWorkspace.tsx, styles.css)
- **Evidence**: live user report on run-306526 / run-297984 — switching
  chat CP05 → CP04 hung the renderer and painted "Task 1/4 Task-042" while
  the run was really at 3/4; the attention inbox popover clipped off the
  window's left edge; the workflow select showed `vibe-adopt-sprint` and
  `vibe-sprint` as user options; "dispatch attention" rows flooded the
  inbox; no worktree control exists anywhere in workflow mode.
- **Root causes / fixes**:
  1. `openHistoryRun` reset `_runReplaySeq[runId]` then called
     `startOrchestrationStream`, which derived `afterSeq` from that
     (empty) map → the SSE restarted at seq 0 on every reopen. On a
     12-hour run, every backlog `agent_graph_updated` overwrote
     `agentGraphSnapshot` (visible sprint regression) and fired
     `refreshWorkflowStepRuntime` + `refreshAgentRuns` per event —
     hundreds of HTTP calls pinned the renderer while the buffered
     transcript replay still hadn't flushed. Fix: reopen path passes
     `handle.lastEventSeq` as the stream's start seq; a seq guard in the
     consumer drops any residual below-boundary frame the server still
     sends. Transcript replay from 0 is unchanged — it IS the backlog
     renderer and its buffer-until-boundary flush is correct.
  2. `.attention-inbox-pop` was `position:absolute; right:0` relative to
     the inbox **button**, which sits mid-header — a 560px panel
     right-aligned to it overflowed ~40px off the window's left edge.
     Re-anchored the containing block to `.app-header`
     (`position:relative` moved up) so the popover's right edge tracks
     the window edge; no other absolute child of `.app-header` exists.
  3. Reopened runs resolve their durable `flowRef` to catalog rows for
     engine-mounted flows (`vibe-sprint`, `vibe-adopt-sprint`,
     `vibe-owner-debate`) and ChatWorkspace appended that row into the
     select options (BUG-1199 mechanism). Fix: `isSystemVibeFlowId` +
     `workflowSelectOptions` keep the state resolution but refuse to
     append system rows; the select shows a placeholder and a read-only
     "Running: <name> (internal)" caption. Non-system non-startable rows
     still append (BUG-1199 preserved).
  4. `settle_pending` is self-clearing terminal bookkeeping (turn
     settlement draining), not a human decision — but it was listed into
     the global attention inbox via both `ingestDispatch` and mux-lane
     `dispatch_attention` decisions, which is why "dispatch attention"
     showed constantly. Both producers now skip `settle_pending`;
     `uncertain`/`repair_required`/`cancel_required` still surface, and
     `DispatchAttentionCard` keeps reading the raw list for ops detail.
  5. `RunInWorktreeToggle` existed only inside `ChatPosturePanel`, which
     returns `null` in workflow mode — workflow runs could only get a
     worktree via direct API calls. The toggle now renders in
     `WorkflowControlPanel` with the same disable rules (running run,
     non-git project, active/merge-pending binding) and the worktree
     path ellipsizes RTL so the tail stays visible.
- **Tests**: 3 new additive test files — `store.openHistoryRun-boundary`
  (start-seq + never-paints-stale-snapshot), `workingMode.workflowSelect`
  (system-flow filtering + BUG-1199 preservation), `attentionQueue.
  settlePending` (both producers filtered, actionable kinds retained).
- **Verification**: `tsc -p tsconfig.phase1-tests.json` clean; phase1
  suite 772 pass / 25 fail — all 25 identical to the pre-change baseline
  (supervisor.js module resolution, experimental localStorage,
  styles.tokens offenders, jira/replay-order flakes); zero regressions.
