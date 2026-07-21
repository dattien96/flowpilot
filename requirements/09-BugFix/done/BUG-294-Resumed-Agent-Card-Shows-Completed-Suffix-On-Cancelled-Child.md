# BUG-294: Resumed Agent Card Shows "— completed" Suffix On A Cancelled Child

## Metadata

- Document ID: `BUG-294`
- Title: `Resumed parent AgentTimelineCard shows "— completed" on a child whose status is cancelled`
- Phase: `bugfix`
- Status: `done`
- Owner: `agent-flow-engine`
- Reviewers: `agent-flow-engine`
- Created: `2026-07-20`
- Last Updated: `2026-07-20`
- Parent Documents: `CP-51-PhaseAB-Timeline-And-Verification-Log`
- Child Documents: `-`
- Related Documents: `BUG-295-Claude-Session-Rotation-Loses-Turn-History-On-Replay` (same run-12255/run-12260 repro; found together, independent root causes)
- Replaces: `-`
- Tags: `agent-flow-engine, chat-ui, transcript-replay, restart, regression, review-loop`

## AI Quick View

### Summary

- After a runner restart, the parent's `AgentTimelineCard` for a child (coder) run showed the text `coder · Claude · cancelled · claude-sonnet — completed` — two status labels that contradict each other on the same card.
- `cancelled` is the child's correctly normalized live status (the child was still running mid-turn when the server was restarted).
- `— completed` is appended purely because the card's `finalMessage` field is non-empty, regardless of whether the child ever actually reached a terminal completed state.
- Root cause: `resumedParentAgentAnnotations` builds the `EventAgentResultInjected` annotation (which sets `finalMessage`) off `child.lastMessage != ""` alone, never checking `child` session `Status`. A run that was merely mid-conversation (has produced at least one assistant message) but got killed by restart still has a non-empty `LastMessage`, so it is unconditionally annotated as if it delivered a final result.

### Current Ask

- Only inject `EventAgentResultInjected` (and thus the "— completed" suffix) for a child whose persisted session status is genuinely terminal-completed (or a terminal state the live flow treats as "delivered a result"), not merely because some chat text exists.

### Key Decisions

- `V-1` The resume path (`resumedParentAgentAnnotations`) emits `EventAgentResultInjected` ONLY for a child whose persisted `session.Status == RunStatusCompleted` (and with a non-empty `LastMessage`) — mirroring the LIVE emit condition exactly (`interactive_service.go`: the annotation fires only for `completion.status == RunStatusCompleted`, and a failed child errors out before reaching it).

### Constraints

- Fix must not regress the legitimate case: a child that DID complete before restart must still show "— completed" on replay (this is the common, correct case exercised by existing tests, e.g. `run2334_restart_transcript_response_test.go`).
- additive-tests-only: any future fix must add new dedicated tests only.

### Open Questions

- RESOLVED: the gating condition is `session.Status == RunStatusCompleted` exactly. Traced the LIVE call site (`interactive_service.go:5162-5171`): the annotation fires only inside `if completion.status == RunStatusCompleted`, and a failed completion returns an error at line 5158-5159 before reaching the annotation. So `RunStatusFailed` is NOT annotated live either — the resume path mirrors "completed only".

### Source Refs

- run-12255 (parent, project db51ec26-1a0f-4b92-8ceb-b03dc8e9b363), child run-12260 (coder).
- `.flowpilot/chats/sessions.ndjson` line 232 (run-12260's last persisted record: `"status":"running"`, non-empty `last_message`).
- Screenshots: FlowPilot Desktop history/transcript view after restart, showing the contradictory card text.

## 1. Issue Summary

A review-loop chat (run-12255) spawned a coder child (run-12260). The child hit a flow-gate regression, went through a remediation retry cycle, and was still waiting on a write-permission approval when the runner was restarted. Post-restart, the parent's transcript replay renders the coder's `AgentTimelineCard` as:

```
coder · Claude · cancelled · claude-sonnet — completed
```

`cancelled` and `— completed` are two claims about the same child run that cannot both be true: a run that is cancelled did not complete.

## 2. Parent Links

- impacted coding plan: `CP-51-PhaseAB-Timeline-And-Verification-Log`
- impacted tech design: agent-flow-engine parent/child transcript replay (`resumedParentAgentAnnotations`)
- impacted system spec: `AgentTimelineCard` (`apps/desktop-flowpilot/src/components/Timeline.tsx:529-551`)

## 3. Environment and Reproduction

- environment: local runner `127.0.0.1:4318`, desktop FlowPilot, Claude provider (claude-sonnet main + coder), Review Loop built-in orchestration, workspace `D:\working\gate-sandbox`.
- reproduction steps:
  1. Start a review-loop bug-fix task that triggers a flow-gate regression and a "Fix the code" remediation retry (as in run-12255 / run-12260).
  2. While the coder child is mid-turn (has produced at least one assistant message, e.g. "The write is still blocked pending your approval...") but has NOT reached a terminal completed/failed state, kill/restart the runner process.
  3. Reopen the parent chat (run-12255) from history.
  4. Observe the coder's `AgentTimelineCard`: status reads `cancelled` (correct) but the card also appends `— completed` (incorrect).
- frequency: deterministic for any child killed by restart after producing at least one assistant message but before reaching a terminal state.

## 4. Expected vs Actual

- expected: a child that never reached a terminal completed state must not display "— completed" on its resumed card. Only `cancelled` (or whatever the normalized terminal status is) should show.
- actual: the card shows both `cancelled` and `— completed` simultaneously — self-contradictory.

## 5. Impact

- users affected: anyone whose flow child is killed by a runner restart mid-turn, in any flow that uses `AgentTimelineCard` (review loop and other flow-engine orchestrations).
- workflows affected: agent-flow-engine child-spawn transcript replay.
- severity: low-medium — cosmetic/trust issue (misleading status), no data loss and no functional breakage of the underlying run state (the child's own `status` field is correctly `cancelled`; only the display annotation is wrong).

## 6. Root Cause

- hypothesis: the "— completed" suffix and the "cancelled" status label are computed by two independent, disconnected mechanisms that were never reconciled.
- confirmed cause:
  - `AgentTimelineCard` (`Timeline.tsx:533,544`): `status = run?.status ?? ...` (the LIVE, correctly-normalized run status) drives the `· {status}` segment; `it.finalMessage` (a property of the PARENT's own "agent" timeline item, set only by an `EventAgentResultInjected` event) independently drives the `— completed` suffix. The two are never cross-checked against each other.
  - `resumedParentAgentAnnotations` (`interactive_resume.go:1980-2038`) is what synthesizes `EventAgentResultInjected` on replay. Its only gate is `if child.lastMessage != ""` (line 2028) — it reads `session.LastMessage` (interactive_resume.go:2009) but never reads or checks `session.Status`.
  - run-12260's last persisted session record (`sessions.ndjson` line 232) has `"status":"running"` and a non-empty `"last_message"` (the assistant's last in-flight remark, not a final result). Because `lastMessage` is non-empty, the annotation fires regardless of the run never having reached a terminal completed state.
- evidence: `.flowpilot/chats/sessions.ndjson` line 232 for run-12260 (`status: running`, non-empty `last_message`); `interactive_resume.go:2020-2038` (`resumedParentAgentAnnotations`); `Timeline.tsx:529-551` (`AgentTimelineCard`).

## 7. Fix Strategy (applied)

- `F-1` — gate the `EventAgentResultInjected` annotation in `resumedParentAgentAnnotations` (`interactive_resume.go`) on `session.Status == RunStatusCompleted` in addition to the existing `LastMessage != ""` check. A `completed bool` field was added to the internal `childSession` struct, populated from `session.Status == RunStatusCompleted`, and the result-annotation append is now `if child.completed && child.lastMessage != ""`. Spawn annotations (`EventAgentSpawnedByUser`) are unchanged — every child still gets its card; only the false "— completed" suffix is removed for non-completed children.

## 8. Validation

- `go test ./internal/runner -run "TestBug294" -count=1` → 3 passed (2 new dedicated tests + suite).
- `go test ./internal/runner -run "TestBug294|TestRun1264Restore|TestRun1264Settle|TestRun2334RestartReplayKeepsAssistantResponsesForEveryProvider" -count=1` → 12 passed (existing restore/annotation/replay coverage stays green).
- Full-package `go test ./internal/runner` residual failures are pre-existing environmental cases (Codex/Gemini CLI binaries, machine home paths); none in the annotation/restore area.

## 9. Regression Guard

- tests: `bug294_resumed_agent_card_completed_suffix_test.go` (additive):
  - a genuinely completed child (status completed, non-empty message) still gets the result annotation → "— completed" preserved (non-regression);
  - a child killed mid-turn (status running, non-empty message) gets NO result annotation → no false "— completed" next to its cancelled status (the bug);
  - a completed child with an EMPTY final message is also skipped (mirrors the live `FinalMessage != ""` guard).
- alerts: none.
- audit checks: `CA-372-resumed-agent-card-completed-parity` ledger block (feature_key `agent-flow-engine`).

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: the LIVE emit path (`interactive_service.go:5162-5171`) is already correct; only the resume reconstruction was brought into parity with it. The desktop `AgentTimelineCard` (`Timeline.tsx:544`) is left as-is — its `— completed` suffix is now driven only by genuinely-completed annotations, so no UI change is needed.
