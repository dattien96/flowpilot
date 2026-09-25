---
id: CA-921b
title: Settle/resume/park fixes — CP-ingest input gate, sprint-state durability, dead gate decisions, stale blockReason + quiet-loop re-drive, durable terminal retry, contract-scope reprompts (BUG-399, 401, 402, 403, 404, 410, 411, 424, 432, 437)
type: BugFix
feature: agent-flow-engine
date: 2026-09-23
status: done
---

## Context

Live-verification wave found ten defects in the settle/resume/park machinery:
`vibe-cp-ingest` ran a plain README through intake to SS drafts; a debate-synthesis
done edge dropped the parked sprint topology; the hub could self-settle `done`
mid-sprint; a missing-verdict reprompt never armed reinvoke recovery; the parked
sprint topology/batch state was RAM-only and lost on restart; a SIGKILL'd flow
came back `cancelled` with a stale `hub_stalled` reason and every resume surface
inert while stale terminal commits were wake-marker no-ops; `SubmitGateDecision`
accepted remediation options on a parked debate into a goroutine that swallowed
the `flow_awaiting_user` 409; delegate retry prompts dropped the frozen
change-contract scope; terminal flow completion left waiting-user children and
stale gate blocks behind; and a flow-control decision that parks cancelled the
very turn submitting the verdict.

## Changes

### BUG-399 — vibe-cp-ingest input shape gate

`internal/runner/vibe_cp.go` + `interactive_service.go`: `startTurn` admission
now validates the flow-starting turn of `vibe-cp-ingest` (turnCount==0,
non-restored) — the source must resolve to a
`requirements/07-Coding-Plan/**/CP-*.md` path (SourceDocID or first CP-shaped
prompt token, `@` stripped) whose file exists and carries `Document ID: CP-*`.
Fail-closed: no source / unreadable / missing Document ID → typed
`invalid_cp_source` 422 before `startResolvedFlow` runs. TUI-side
`DetectVibeEntry` still gates the picker; this closes the direct-API bypass.
Tests: `TestBug399_VibeCpIngest*` (reject README prompt / missing Document ID /
missing file, accept real CP, follow-up turn not revalidated).
Live-verified: `POST /turns` with `README.md` → 422; with a real CP → admitted,
`cp_reader` spawned.

### BUG-401 — debate-synthesis done edge restores parked sprint

`internal/runner/interactive_service.go` (`advanceHubDoneThroughEdge` terminal
edge) + `interactive_handlers.go` (HTTP done routes through the edge): when a
flow-control done arrives while a vibe sprint is parked behind a debate, the
stashed `vibeParkedNodes`/`Edges`/`Acceptance`/`FlowRef` are restored before
whole-run settle instead of silently dropping them.
Tests: `TestBug401_DebateSynthesisDoneRestoresParkedSprint`,
`TestBug401_NoStashedSprint_FallsThrough`.

### BUG-402 — refuse agent-initiated done while sprint tasks remain

`internal/runner/interactive_service.go` + `agent_orchestrator.go`: new
`FlowControlInput.agentInitiated` set inside `turnBridge.SubmitFlowControl`
(the single agent funnel for `flow_control` and mapped `submit_review_outcome`).
The `done` case refuses with escalate when `vibeTaskPlan - vibeSprintIndex > 0`
AND the verdict is agent-initiated (live run-9/run-2290 settled 1/3). Operator
settles (boundary cancel/decline via `declineVibeSprintBoundary`, HTTP
flow-control) are unflagged and still complete — refusing them would wedge the
gate (caught by `TestVibeSprintBoundary_*CancelSettlesDone/Decline*`).
Tests: `TestBug402_DoneRefusedWhileSprintTasksRemain` (agent refused+escalated;
operator done publishes).

### BUG-403 — verdict reprompt arms reinvoke recovery

`internal/runner/interactive_service.go`: `reinvokeInFlight` is set BEFORE
`scheduleChildTurn` on the missing-verdict reprompt path, so a
`turn_in_progress` rejection re-arms + drains instead of leaving the hub
wedged with no recovery marker.
Test: `TestBug403_VerdictRepromptArmsReinvokeRecovery`.

### BUG-404 — parked sprint topology persists

`internal/runner/interactive_service.go` + `interactive_resume.go` +
`sessions.go`/`workflow_store.go`/`local_file_session_store.go`: `vibeParkedNodes`,
`vibeParkedEdges`, `vibeParkedAcceptance`, `vibeParkedFlowRef` and
`pendingBatchSignatureByStep` now round-trip through `ProviderSessionState`, so a
restart mid-debate-negotiation rebuilds the sprint topology instead of
false-done'ing the flow.
Test: `TestBug404_ParkedSprintStateRoundTripsSession`.

### BUG-410 — post-SIGKILL recovery + bounded stale terminal commit

`internal/runner/interactive_resume.go`: `loadPersistedRun` sanitizes a stale
`BlockReason` when the restored loop is not `blocked` (crash stamped
`status=cancelled` + `blockReason=hub_stalled` on a `running` loop).
`internal/runner/interactive_service.go` + `interactive_handlers.go`: new
`redriveQuietFlowLoop` — when a resumed flow parent's loop reads `running`
but nothing is in flight, queued, parked, or pending anywhere, the hub is
re-driven (`autoOrchestrate` re-armed, crash `cancelled` healed to running).
Wired into `resumeAgentLoop` and `/resume`.
`internal/runner/dispatch_live.go`: `reconcileDispatchTurn` is no longer a
wake-marker no-op — a stale `CommitTerminalAndSettleIntent` / bridge terminal
now re-reads the durable record and retries the CAS in a bounded loop instead
of silently dropping terminalization.
Tests: `TestBug410_ReconstructClearsStaleBlockReason`,
`TestBug410_ResumeRedrivesQuietMidFlightFlow`,
`TestBug410_TerminalCommitRetriesOnStaleRevision`,
`TestBug410_BridgeTerminalRecoversFromStaleCAS`.
Note: first implementation deadlocked on `loopAllowsNextTurnLocked` (helper
re-locks `s.mu` in the blocked:paused branch — caught by
`TestCA801_InMemoryResumeParksConfirm` hanging); the advancing check is now
inlined under the already-held lock with identical semantics.

### BUG-411 — gate decision on a parked loop is the human decision

`internal/runner/gate_hook.go` (`SubmitGateDecision`): when the target run's
own loop — or its parent's — is `blocked` awaiting a decision, the chosen
option is routed into `resumeFlowWithFeedback` so the parked hub re-drives with
the remediation in its reinvoke note, instead of `go startTurn()` hitting the
`flow_awaiting_user` fence while the goroutine swallowed the error and
`pendingGateBlock` was already consumed (accepted-and-dead). Dispatch rejection
now restores `pendingGateBlock` and returns the real error.
Tests: `TestBug411_GateDecisionOnParkedChildRoutesToParentResume`,
`TestBug411_GateDecisionOnBlockedHubResumesFlow`,
`TestBug411_GateDecisionSurfacesStartTurnReject`.

### BUG-424 — resume retry prompt carries the change contract

`internal/runner/interactive_service.go` (`resumeFlowWithFeedback`): all three
delegate-respawn prompts (reinvoke failed child, fresh respawn, missing
change-audit writer retry) now run through
`appendChangeContractIfAnyWithSecret(workspaceCwd, parentRunID, prompt,
markerSecret)` so the retried writer sees the frozen scope block, not just
the user's feedback text.
Test: `TestBug424_ResumeRetryPromptCarriesContractScope` (asserts the
`flowpilot-cc:` marker block, proven red on baseline).

### BUG-432 — terminal flow completion reconciles children + gate state

`internal/runner/flow_step_runtime.go` + `interactive_service.go`: flow `done`
now reconciles `waiting_user_approval` children to completed, clears a stale
`pendingGateBlock`, and the spawn path refuses new children while the parent
loop is `blocked` — stale gate-decision state can no longer trigger remediation
on a dead loop.
Tests: `TestBug432_ReconcileSettlesWaitingUserApprovalChild`,
`TestBug432_FlowDoneClearsPendingGateBlock`,
`TestBug432_GateDecisionRejectsWhenNothingPending`,
`TestBug432_SpawnRefusedWhileParentLoopBlocked`.

### BUG-437 — park preserves the decision-submitting turn

`internal/runner/interactive_service.go` (`parkFlowForAwaitingUser` + locked
variant): a park triggered synchronously by the hub turn's own
flow_control/submit_review_outcome call preserves that turn so the provider
can finish returning its verdict; unrelated in-flight turns are still
cancelled and `parkCancelCause`/`parkCancelSuppress` stay armed.
Mechanism: dedicated `interactiveRun.parkPreserveTurnID` marker armed only at
the agent funnel (`turnBridge.SubmitFlowControl` → `agentInitiated`), checked
in both park functions. It deliberately does NOT reuse `lastFlowControlTurnID`
— that stamp is also set by engine-internal `applyFlowControl` calls (audit
escalate, machine-verdict credit, plan-approval), and keying preserve on it
regressed `TestRun203966AuditEscalateParkKeepsParentNonterminal` (engine
escalate must still cancel an unrelated in-flight hub turn). Caught by the
full-suite delta vs baseline and corrected before close-out.
Tests: `TestBug437_ParkPreservesFlowControlSubmittingTurn`,
`TestBug437_ParkStillCancelsNonDecisionTurn` (now also pins that an
engine-stamped `lastFlowControlTurnID` alone does NOT preserve),
`TestRun203966*` suite re-green.

## Provider parity

All touched seams are provider-agnostic runner code — admission gate,
`applyFlowControl`, `SubmitGateDecision`, `resumeAgentLoop`/`resumeRun`,
`reconcileDispatchTurn`, park/cancel — no adapter, event-stream, or
provider-session code changed. Tests exercise `ProviderKeyCodex`; the
mechanisms sit below provider dispatch. Live verification ran on the local
runner binary (BUG-399 HTTP repro).

## Verification

- Focused: `go test -count=1 -run 'TestBug39|TestBug40|TestBug41|TestBug42|
  TestBug43|VibeSprintBoundary|TestCA801' ./internal/runner/` — all green
  (24 Cluster F tests).
- Baseline redness: all reproduction tests verified failing by assertion (or
  compile-red for new symbols) on the clean baseline worktree.
- Full-runner suite delta vs clean `d191004f` baseline: identical pre-existing
  failure set (BUG-427 + env deps); one real regression found and fixed
  (BUG-437 preserve initially keyed on `lastFlowControlTurnID` broke
  `TestRun203966AuditEscalateParkKeepsParentNonterminal` — corrected to the
  dedicated `parkPreserveTurnID` marker); remaining delta tests are load-only
  flakes passing in isolation on both trees.
- Live: local runner serve, `flowRef=vibe-cp-ingest` + `README.md` →
  `422 invalid_cp_source`; real CP → admitted + cp_reader spawned.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-410
change_type: bugfix
summary: Cluster F settle/resume/park fixes — CP-ingest input gate, sprint durability, dead gate decisions, crash re-drive, terminal-commit retry, contract-scope reprompts
# --->8---
