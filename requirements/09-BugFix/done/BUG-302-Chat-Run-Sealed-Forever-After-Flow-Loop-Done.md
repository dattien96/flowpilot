# BUG-302 — Chat run sealed forever after flow loop reaches "done"

## Metadata

- Document ID: `BUG-302`
- Title: Chat run sealed forever after flow loop reaches "done"
- Phase: `bugfix`
- Status: `done`
- Owner: local-runner
- Reviewers: n/a
- Created: 2026-07-21
- Last Updated: 2026-07-21
- Parent Documents: CP-36 (Agent Review Loop And Main Hub Orchestration), CP-51-PhaseAB-Timeline-And-Verification-Log.md §3.1 A10
- Child Documents: none
- Related Documents: BUG-288 (gate lifecycle re-entry, original source of the guard), BUG-226 (submit_review_outcome fallback escalate), run-18997
- Replaces: none
- Tags: chat-ui, agent-flow-engine, regression

## AI Quick View

### Summary

- A run (Chat mode **or** Workflow mode) whose flow finished successfully (`loop_state.Status == "done"`) could never receive another message on the same run — every subsequent `startTurn` was rejected with `409 flow_stopped`, identical to a genuinely `Stop`-ped run.
- Confirmed live on run-18997 (CP-51 A10 test): flow reached `done` (approved) after round 1; sending a plain follow-up ("hello, turn trước bạn fix gì thế") was rejected outright, with no way to continue the conversation on that run.
- This contradicts CP-36's own design ("Loop ends. Board shows done. No further spawns." — the FLOW stops acting, not the chat) and the desktop's own "Chat Intent locked after the first message" contract, which only makes sense if the same run stays usable afterward. It also contradicts the desktop's own composer (`ChatInput`): the same component is rendered unconditionally for every run regardless of Chat vs Workflow mode, with no distinct "closed" affordance for either — confirmed by reading `ChatWorkspace.tsx`'s single, unconditional `<ChatInput />` usage.
- A naive fix (just relax the admission gate) would have traded this bug for another: `offerReviewOutcomeTool`/BUG-226's "hub completed without calling submit_review_outcome → escalate" fallback would misfire on the very next plain-prose follow-up, incorrectly reopening a "Needs your decision" card.

### Current Ask

- Let **any** run (chat or workflow) keep accepting normal follow-up messages after its flow loop reaches `done`, without reopening any flow-decision machinery (review-outcome tool offering, BUG-226's escalate fallback) for those follow-ups, and without weakening the existing `stopped`/`blocked` seals.

### Key Decisions

- `V-1` (revised — see below) Initially scoped the relaxation to `run_kind == "chat"` only, reasoning a Workflow-mode run was a one-shot execution with no follow-up contract. **Reverted after user correction**: the desktop's composer (`ChatInput`) is the identical component for both modes with no "closed" state either way, so the carve-out applies to every `run_kind` — only `"stopped"` seals a run now; `"done"` never does, regardless of chat vs workflow.
- `V-2` Capture whether the loop was already `done` **before** this turn started, and use that to suppress `offerReviewOutcomeTool` (and by construction, the BUG-226 fallback gated on it) for that turn — a follow-up turn that begins after the flow already settled must never be mistaken for the hub's own review-decision turn.

### Constraints

- Must not touch the `stopped`/`blocked` seals, the child-run branch of the same admission gate, or any BUG-288-hardened reprompt/park logic.
- additive-tests-only: only new test files/cases; no existing test edited.
- cross-provider-parity: verify whether the touched code branches on `providerKey`.

### Open Questions

- None.

### Source Refs

- run-18997 (`.flowpilot/chats/sessions.ndjson`, `loop_state.status == "done"`, rejected follow-up)
- `apps/local-runner/internal/runner/interactive_service.go` (`startTurn`'s admission gate, `offerReviewOutcomeTool`)
- CP-36 §"Loop ends. Board shows done. No further spawns."

## 1. Issue Summary

While manually running CP-51's A10 test (send a follow-up user message on the same hub after its Review Loop flow completes), the desktop showed `flow loop is stopped; cannot start a new turn (HTTP 409 / flow_stopped)` for a flow that had actually finished **successfully** (`done`, approved), not been stopped.

## 2. Parent Links

- impacted coding plan: CP-36 (review loop design), CP-51 (durable turn dispatch, verification log)
- impacted tech design: n/a
- impacted system spec: n/a

## 3. Environment and Reproduction

- environment: local dev, Claude provider, Chat mode, Review Loop built-in orchestration
- reproduction steps:
  1. Start a Chat→Bug Review Loop run and let it reach `done` (approved).
  2. Send any new message on the same run (e.g. a plain question about what happened).
- frequency: deterministic for every chat run whose flow reaches `done`

## 4. Expected vs Actual

- expected: the run keeps accepting normal chat turns after its flow completes (CP-36: the flow stops acting; the chat does not).
- actual: every subsequent turn is rejected with `409 flow_stopped`, indistinguishable from a genuinely stopped run.

## 5. Impact

- users affected: anyone continuing a conversation on a Chat-mode Review Loop run after it completes
- workflows affected: Chat-mode built-in orchestration (Review Loop and any future chat-selectable flow)
- severity: medium (the flow's own result is unaffected, but the chat session becomes permanently unusable for follow-up — no in-app recovery short of starting a new run)

## 6. Root Cause

- hypothesis: an admission-gate check conflates "the flow's own decision loop is over" with "this chat run must never receive another message."
- confirmed cause: `startTurn`'s root-run branch (`apps/local-runner/internal/runner/interactive_service.go`, originally added in commit `968d210` for BUG-288) rejects any new turn when `loopStateFor(rs.id).Status` is `"stopped"` **or** `"done"`:
  ```go
  if st := s.agentOrchestrator.loopStateFor(rs.id).Status; st == "stopped" || st == "done" {
      return "", newAPIErr(http.StatusConflict, "flow_stopped", "flow loop is stopped; cannot start a new turn")
  }
  ```
  The surrounding comment only explains the `"stopped"` case (Stop creates an intent race with in-flight reinvoke/gate work) — no rationale is given for including `"done"`, and the flow-launch-from-chat-flowRef logic (`resolveWorkflowFlowRef`-equivalent, chat's own `in.FlowRef` branch) is independently already gated to `rs.turnCount == 0`, so relaxing `"done"` cannot reopen the "blindly relaunch the flow" risk the guard was originally worried about.
- evidence:
  - Live run-18997: `loop_state.status == "done"` (persisted), follow-up turn rejected with `flow_stopped`.
  - New regression test `TestChatFollowUpAllowedAfterFlowLoopDone` reproduces the exact scenario and fails against the unfixed code (confirmed via git-stash) with the same rejection.
  - Naive-fix risk confirmed by reading `offerReviewOutcomeTool := rs.autoOrchestrate && rs.parentRunID == "" && rs.turnCount > 1` (unconditional on turnCount alone) combined with BUG-226's "hub synthesis turn completed without calling submit_review_outcome → escalate" fallback — both would still fire on a plain-prose follow-up turn if only the admission gate were relaxed, wrongly re-escalating a "Needs your decision" card for a completed flow.

## 7. Fix Strategy

- `F-1` In `startTurn`'s root-run admission gate, only `"stopped"` rejects a new turn; `"done"` never does, for any `run_kind`. `"blocked"` and the child-run branch are unchanged.
- `F-2` In `runTurn`, capture `loopAlreadyDoneAtTurnStart` (the loop's status at the very start of this turn, before anything this turn does can change it) and AND it into `offerReviewOutcomeTool`'s computation, so a turn that begins after the flow already settled never offers/requires `submit_review_outcome` and never trips BUG-226's escalate fallback (which is itself gated on `offerReviewOutcomeTool`).

## 8. Validation

- `V-1` **cross-provider-parity (Case 1, provider-agnostic — confirmed by reading, not assumed):** both touched sites — the loop-status admission gate and the `offerReviewOutcomeTool` computation — read only `rs.runKind`, `rs.turnCount`, `rs.autoOrchestrate`, `rs.parentRunID`, and `agentOrchestrator.loopStateFor(...).Status`; none take or branch on `providerKey`. One representative-provider test (Codex, matching this file's existing sibling pattern) is sufficient per the skill's Case-1 allowance.
- `V-2` **additive-tests-only:** new file `apps/local-runner/internal/runner/bug302_chat_followup_after_flow_done_test.go` adds three tests only; no existing test file was edited.
  - `TestChatFollowUpAllowedAfterFlowLoopDone` — the fix scenario on a chat-kind run; also asserts the loop stays `done` and the run does not flip to `waiting_approval` after a plain-prose follow-up (proves F-2, not just F-1).
  - `TestWorkflowRunAlsoAllowsNewTurnAfterFlowLoopDone` — the same scenario on a `run_kind == "workflow"` run, confirming the carve-out is not chat-only (see the reverted `V-1` above).
  - `TestChatRunStillRejectsNewTurnWhenStopped` — regression guard: a chat run's genuinely `"stopped"` loop still seals it.
  - Confirmed via git-stash: both `TestChatFollowUpAllowedAfterFlowLoopDone` and `TestWorkflowRunAlsoAllowsNewTurnAfterFlowLoopDone` fail against the unfixed code (`flow_stopped` rejection reproduced) and pass with the fix; the `"stopped"` regression-guard test passes unchanged either way.
- `V-3` **Full-suite regression, baseline captured before any change:** `go test ./internal/runner/ ./internal/flowgate/ ./internal/agentpack/ -count=1 -timeout 10m` — baseline (pre-fix): **2199 passed, 20 failed** (each of the 20 individually classified: 16 environment-dependent — missing `codex`/`git` binaries, Windows-specific home-dir resolution, POSIX-shell mocking that doesn't work on native Windows — 1 stale assertion (`TestResolveGoogleDriveMcpProviderStatuses_AllNotStarted`, predates Grok's addition as a 4th provider), and 1 unrelated real bug in transcript-merge dedup (`TestMergeTurnLogAssistantsIntoTranscriptAddsMissingHubSynthesis`); none touch this fix's code paths). After the fix + 3 new tests, across repeated runs: **2201–2203 passed, 19–21 failed**, always the same-or-narrower set as the baseline plus known pre-existing flakes. Two tests surfaced as occasional extra failures across different runs — `TestStartTurnGrokSameAccountFollowUpUsesPromotedRealID` (Windows temp-dir cleanup race) and `TestNonCohortPreflightStartTurnFailReinvokesHub` (a genuine pre-existing async race between a synchronous `pendingAgentContext` read and a `go`-spawned reinvoke goroutine draining it) — both were confirmed, by running each 8× in isolation against **both** the fixed and unfixed code, to fail at a statistically identical rate either way (Grok: fails every time either way; NonCohortPreflight: ~25–35% failure rate either way). Pre-existing flakiness, not a regression from this fix.
- `V-4` Targeted re-run of 93 gate/flow-control/reinvoke/settle-related tests (`TestCohort*`, `TestApplyFlowControl*`, `TestResumePendingFlowGate*`, `TestChildGate*`, `TestBuiltinOrchestration*`, `TestRun1264*`, `TestRun1618*`, `TestRun9437*`, `TestBug288*`/`Test.*[Bb]ug288`, `TestBug298*`, `TestRun2383*`, `TestGateSettle*`, `TestFinishTurn*`, `TestStartTurn*`, `TestEscalate*`, `TestRun2047*`, `TestRun5296*`) — all 93 pass unchanged.
- `go build ./...` passes.

## 9. Regression Guard

- tests: `TestChatFollowUpAllowedAfterFlowLoopDone`, `TestWorkflowRunAlsoAllowsNewTurnAfterFlowLoopDone`, `TestChatRunStillRejectsNewTurnWhenStopped`.
- alerts: n/a
- audit checks: none beyond the existing gate/flow-control battery.

## 10. Follow-Up Document Updates

- upstream docs that must change: CP-51-PhaseAB-Timeline-And-Verification-Log.md §3.1 A10 to be updated with this fix's evidence once A10 is live-retested end-to-end.
- notes left unchanged on purpose: the child-run branch of the same admission gate, `"blocked"` handling, and every other BUG-288-hardened reprompt/park path were left untouched — this fix only carves out the chat-kind "done" case and scopes `offerReviewOutcomeTool` to not apply retroactively to a settled loop.
