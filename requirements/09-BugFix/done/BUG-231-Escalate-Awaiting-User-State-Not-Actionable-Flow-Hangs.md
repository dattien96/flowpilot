# BUG-231: Escalate / Awaiting-User Flow State Is Not Actionable — Synthesis Step And Main Chat Hang

## Metadata

- Document ID: `BUG-231`
- Title: `Escalate / Awaiting-User Flow State Is Not Actionable — Synthesis Step And Main Chat Hang`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-03`
- Last Updated: `2026-07-03`
- Parent Documents: [Task-183: User-Owned Model/Provider In Flow Mode](../../08-Task/todo/Task-183-User-Owned-Model-Provider-Resolution-Across-Chat-And-Flow.md), [CP-36: Agent Review Loop And Main Hub Orchestration](../../07-Coding-Plan/inprogress/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [CA-226: Hub Synthesis Fallback Escalation](../../../change-audit/CA-226-hub-synthesis-fallback-escalation.md)
- Child Documents: `none`
- Related Documents: [BUG-228: Agent-Delegate Flow Node Ignored Its Own Step Model Mid-Flow](../done/BUG-228-Step-Level-Model-Not-Reresolved-Mid-Flow.md)
- Replaces: `none`
- Tags: `agent-flow-engine, review-loop, flow-control, escalate, desktop, step-timeline, hang`

## AI Quick View

### Summary

- User ran the built-in "Review Loop" with **mixed reviewer verdicts** (one reviewer approved, one requested changes). The final `synthesis` (hub inline) step stays visually RUNNING and the main chat stays stuck on "Waiting for the current turn…" — the run never finishes. This happens after the synthesizer submits a `blocked` review outcome.
- **This is NOT a bug in the escalate *semantics*.** `submit_review_outcome(status="blocked")` maps to flow-control `escalate`, which by design sets the loop to `blocked` and returns `NextAction: "awaiting_user"` — i.e. the flow is *supposed* to pause and wait for the human. That is correct.
- **The bug is that the "awaiting_user" state is never surfaced as actionable**, at either tier: the synthesis step-timeline row stays `RUNNING` (a spinner that reads as a hang, not "waiting for you"), and the desktop derives the run status as `running` from a `blocked` loop, which *locks* the main-chat composer ("Waiting for the current turn…") — so the user the flow is waiting on literally cannot respond.
- The **mixed verdict** is what surfaces this: an all-approved run ends in `done` (settles cleanly, works today — `TestE2EReviewLoopApprovedPath`); a mixed verdict legitimately routes to `changes_requested`→`continue` and, once the cap is reached, to `blocked` (interactive_service.go continue-case), or the synthesizer escalates to `blocked` directly — both land in the un-actionable `blocked` state.

### Current Ask

- Implemented (`D-1`..`D-8`, DoD §11 items R1-R6/D1-D6/T1-T3/DOC all done; `Q-3` intentionally deferred — see below). Escalate/cap-reached remain a non-terminal "awaiting user" pause; the pause is now visible (step timeline: WAITING_USER_APPROVAL; desktop: unlocked composer + inline Continue/Stop card) and actionable (Continue lets the hub re-decide; Stop cancels).

### Key Decisions

- `D-1` Escalate → `blocked` / `awaiting_user` is intended behavior and must be preserved. The flow legitimately hands control back to the human (e.g. the synthesizer genuinely cannot decide, or a security concern needs sign-off). The fix must NOT convert escalate into an automatic `done`/`failed` terminal state.
- `D-2` (Superseding an earlier mis-framing) The synthesis hub node must NOT be marked `FAILED` on escalate — `FAILED` reads as an error/terminal step. The correct settlement for an awaiting-user pause is a distinct waiting status (`WAITING_USER_APPROVAL`, which already exists in `RuntimeWorkflowStepStatus`), so the timeline shows "this step is waiting on you" rather than a spinner. **Decided (Q-2)**: reuse `WAITING_USER_APPROVAL` rather than adding a new enum value — it already renders as a paused/awaiting state and avoids new plumbing across Go + contract + desktop.
- `D-3` The desktop must stop mapping loop `blocked` → run `running`. A `blocked`/`awaiting_user` loop unlocks the composer and surfaces a recovery affordance directly in the main chat, not only in the OrchestrationBoard.
- `D-4` **Decided**: the recovery affordance is modeled on the existing `ask_user` / question form — a free-text feedback box plus two buttons, **Continue** and **Stop**. It renders inline in the main chat when the loop is `blocked`/`awaiting_user`. The two blocked reasons (escalate vs cap-reached) are handled by ONE unified Continue action; the user is not asked to pick "extend cap" manually (see `D-5`).
- `D-5` **Continue semantics (decided): "answer the hub, then let the hub route."** Continue does NOT hard-route to the coder. It:
  1. injects the user's feedback (if any) into the hub/parent context,
  2. moves the loop `blocked` → `running`, auto-raising the cap when (and only when) the block reason is cap-reached so the resume cannot immediately re-block (see `D-6`),
  3. re-runs the hub **synthesis** turn with the feedback in context.
  The hub then re-decides via `submit_review_outcome` and picks the route itself: if it can tell which reviewer is right → `changes_requested` → loop back to the coder to update; if it judges the reviewers wrong / no code change needed → `approved` → `done` (flow finishes); if still genuinely stuck → `blocked` again → the form re-appears for another human turn.
- `D-6` **Auto-extend for both cases (decided)**: the user never has to click a separate "Extend cap" button. On Continue, if the block reason is cap-reached, the cap is raised automatically by `ExtendBy` so the resume cannot immediately re-block. Every Continue is a deliberate human click (the form re-appears on every subsequent block), so the human is the real bound.
- `D-7` **Stop** reuses the existing `stopAgentLoop` + interrupt path → loop `stopped`, run settles to `cancelled`, composer unlocks. No new mechanic needed.
- `D-8` **Retire `ExtendMax` as a hard blocker (decided)**. There are two distinct limits and only ONE is still needed:
  - `Cap` (`policy.cap`, default 3) bounds the **auto-loop** — `maybeAutoReinvokeHub` will not fire once `Round >= effectiveCap`, so the loop pauses and hands control to the human via `onCap: escalate`. This is the essential "bring the human in" safety bound and **stays**.
  - `ExtendMax`/`ExtendCount` (`policy.extendMax`, default 2) only ever bounded the **user-triggered** `extendCap` action (verified: `extendCap` is called solely from the HTTP handler / Board button and tests, never from any auto path). Once Continue (a human click) auto-extends and is exempt from it, `ExtendMax`'s only remaining effect would be to block the *user* after 2 extensions → loop stuck at `blocked` → the exact wedge this bug fixes. So `ExtendMax` is retired as an enforced limit. `ExtendCount` may be kept for display/telemetry ("extended N times") but is no longer used to reject an extend.
  - Consequence: the standalone Board "Extend cap +2" button is redundant with Continue's auto-extend; fold it into Continue (or keep only as an unbounded convenience shortcut) so there are not two inconsistent extension paths.

### New runner action required

The runner today has only the disjoint primitives `extendCap`, `injectAgentFeedback`, and `stopAgentLoop`. `D-5`/`D-6` need a single combined "resume-from-blocked with feedback + re-run hub" action (working name `resumeFlowWithFeedback` / a new `agent-loop/continue` endpoint) that performs inject → set-running (+auto-extend if cap-reached) → re-invoke the hub synthesis turn, atomically and idempotently.

### Open Questions

- `Q-3` (Trigger, separate) In the captured run the synthesizer escalated because it reported the joined result note missing ("I don't see a result note in the conversation above"). Static tracing shows that for review-loop the parent turn is idle at cohort-join, so `maybeAutoReinvokeHubWithNote` should take the fast path and embed the note directly in the synthesis prompt — meaning the note *should* be present. Needs a live runner-log repro to determine whether this is (a) a genuine timing race that forces the deferred/empty-note path, or (b) Claude Haiku failing to read the embedded note. Tracked as a contributing trigger, not the core hang — even a correctly-delivered note leading to a legitimate escalate wedges identically.

Resolved: `Q-1` (affordance = feedback box + Continue/Stop, `D-4`); `Q-2` (reuse `WAITING_USER_APPROVAL`, `D-2`); cap handling (auto-extend, `D-6`); Continue routing (hub re-decides, `D-5`).

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go:568-580` (`applyFlowControl` "escalate" case — sets `blocked`, `NextAction: awaiting_user`; no step-timeline settlement, unlike the "done" case at `:498-514` which calls `markFlowRunComplete`)
- `apps/local-runner/internal/runner/interactive_service.go:516-566` (`applyFlowControl` "continue" case — cap-reached also sets `status="blocked"` at `:532-535`)
- `apps/local-runner/internal/runner/interactive_service.go:1496-1498` (cohort join sets the hub inline `synthesis` node to `StepStatusRunning`, never moved off on escalate)
- `apps/local-runner/internal/runner/interactive_service.go:2445-2454` (CA-226 prose-only fallback that DOES settle the hub — skipped here because `submit_review_outcome` was called)
- `apps/local-runner/internal/runner/agent_orchestrator.go:594-606` (`reviewOutcomeFace` — `blocked`→`escalate` mapping)
- `apps/local-runner/internal/runner/workflow_state_machine.go:20-24` (`RuntimeWorkflowStepStatus` incl. `WAITING_USER_APPROVAL`)
- `apps/desktop-flowpilot/src/state/store.ts:2245-2249` (`deriveOrchestrationRunStatus` maps loop `blocked`→run `running`)
- `apps/desktop-flowpilot/src/components/ChatInput.tsx:489,711-714` (`blocked` gate → composer disabled + "Waiting for the current turn…")
- `apps/desktop-flowpilot/src/components/OrchestrationBoard.tsx:49-55,127-130` (only surface of the blocked banner + "Extend cap +2", not in main chat)

## 1. Issue Summary

When the built-in Review Loop ends in a `blocked`/`awaiting_user` state — via a direct `escalate` (synthesizer submits `blocked`) or via `continue` hitting the round cap — the synthesis step stays `RUNNING` on the timeline and the desktop main chat composer stays locked ("Waiting for the current turn…"), with no way for the user to act from the main chat. The awaiting-user pause is real and intended, but it is presented as an indistinguishable hang.

## 2. Parent Links

- impacted coding plan: `CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration`, `CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor`
- impacted tech design: `SD-19-Agent-Flow-Engine` (flow-control terminal/awaiting states), `SD-06-AI-Provider-Integration` (only tangential)
- impacted system spec: `SS-05-Workflow-Ai-Provider` (only tangential)

## 3. Environment and Reproduction

- environment: desktop-flowpilot + local-runner, Flow Mode / chat "bug" sub-mode running the built-in "Review Loop".
- reproduction steps: run Review Loop on any task; have the two reviewers return a mixed verdict (one approve, one changes-requested) so the synthesizer either loops to the cap or escalates to `blocked`; observe the `synthesis` step and the main chat.
- frequency: deterministic whenever the loop ends in `blocked` rather than `done` (mixed verdicts make this the common case; all-approved avoids it).

## 4. Expected vs Actual

- expected: when the flow escalates / awaits the user, the timeline shows the synthesis step as "waiting on you" and the main chat lets the user respond (give guidance and resume, extend cap, or stop).
- actual: the synthesis step shows `RUNNING` (looks like a hang) and the main chat composer is locked with "Waiting for the current turn…", with no reachable action — the run appears frozen.

## 5. Impact

- users affected: anyone running the Review Loop (or any flow whose control tool can return escalate / hit the cap) to a non-approved conclusion.
- workflows affected: every flow that can reach `blocked`/`awaiting_user`.
- severity: high — the flow is un-completable and the UI is wedged from the user's perspective, even though the runner state is internally consistent (correctly awaiting the user).

## 6. Root Cause

- The `blocked`/`awaiting_user` state has no actionable representation:
  - Runner: `applyFlowControl` "escalate" (and cap-reached "continue") set the loop to `blocked` but never transition the hub inline step off `RUNNING`; only the "done" case settles the step timeline (`markFlowRunComplete`).
  - Desktop: `deriveOrchestrationRunStatus` treats loop `blocked` as run `running`, so the composer stays disabled and no terminal/awaiting event unlocks it; the only recovery affordance (`OrchestrationBoard` "Extend cap +2") is off the main chat and is wrong for a genuine escalate.
- Contributing trigger (why escalate fired here): the synthesizer reported the joined result note missing — see `Q-3`.

## 7. Fix Strategy

Implemented per `D-1`..`D-8`; per-DoD-item detail in §12 (all checked off there).
- **Runner — settle the waiting node**: on escalate (and cap-reached "continue") for a flow-engine-driven run, `setFlowStepAwaitingUser` transitions the hub inline node to `WAITING_USER_APPROVAL` instead of leaving it `RUNNING`; the loop stays `blocked`/`awaiting_user` (never auto-`done`/`failed`).
- **Runner — new resume action** `resumeFlowWithFeedback` (`POST .../agent-loop/continue`): injects the user's feedback directly into the re-invoked prompt → sets loop `blocked`→`running` (auto-raises the cap by `ExtendBy` only when `BlockReason=="cap"`) → re-invokes the hub synthesis turn via `maybeAutoReinvokeHubWithNote`. The hub re-decides and self-routes (`D-5`): `changes_requested`→coder, `approved`→`done`, or `blocked`→pause again (idempotent no-op if not currently blocked).
- **Runner — retired `ExtendMax` enforcement** (`D-8`): `Cap` (auto-loop bound) unchanged; `extendCap` no longer rejects on `ExtendCount >= ExtendMax` (counter kept for display).
- **Desktop — status derivation**: `deriveOrchestrationRunStatus` now returns a new `"blocked"` `RunStatus` (distinct from `running`) for a blocked loop, so the composer unlocks.
- **Desktop — inline Continue/Stop form** (`FlowAwaitingUserCard`, modeled on `QuestionCard`/`ask_user`): renders between the timeline and the composer whenever the loop is `blocked`; shows the gate reason, a feedback textarea, Continue (`continueFlow`) and Stop (existing `stop`). `OrchestrationBoard`'s blocked banner was folded into the same `continueFlow` action, removing the now-redundant standalone "Extend cap +2" button (`D-8` consequence).
- **Also found and documented (not the core fix, but adjacent)**: CP-36's original design already called for the hub's *skill* to call `ask_user` on `blocked` — a model-behavior-dependent path with the same reliability gap CA-226 found for `submit_review_outcome`. This fix makes the pause deterministic at the engine layer regardless of whether the skill also calls `ask_user` — see the CP-36 `P-4` note added alongside this fix.
- **Trigger (`Q-3`)**: deferred — separately reproduce and resolve the note-delivery issue with a live runner log. Does not block this fix.

## 8. Validation

- `V-1` New Go tests: `TestApplyFlowControlEscalateSettlesHubToWaitingUser`, `TestApplyFlowControlCapReachedSettlesHubToWaitingUser`, `TestResumeFlowWithFeedbackAutoExtendsOnlyForCap` (both cap and escalate sub-cases), `TestResumeFlowWithFeedbackNoopWhenNotBlocked`, `TestResumeFlowWithFeedbackReinvokesHubSynthesisTurn`, `TestExtendCapNoLongerRejectedPastFormerMax` — all pass; the 5 escalate/resume tests re-run 15x each (`-count=15`, 75 runs total) with 0 failures.
- `V-2` `go build ./...` / `go vet ./internal/runner/...` in `apps/local-runner` — clean. `go test ./internal/runner/...` — 1065 passed, 15 failed (identical pre-existing, environment-specific failure set already established as this session's baseline), 14 skipped.
- `V-3` New desktop tests (`store.test.ts`): `deriveOrchestrationRunStatus` returns `"blocked"` (not `"running"`) for a blocked loop, including when `BlockReason` is `"cap"`/`"escalate"`/unset, and confirms an actively-running child still wins over a blocked loop; `continueFlow` calls `client.continueFlow` with the trimmed feedback and applies the returned snapshot; `continueFlow` is a safe no-op when the client doesn't implement it.
- `V-4` `npm run typecheck` and `npm run build` in `apps/desktop-flowpilot` — clean/pass. Full desktop unit suite (`AgentsPanel`, `navigatorHistory`, `timelineGrouping`, `normalizeImage`, `store`, `timelineReducer`, `navigatorCatalog`) — 116 passed, 2 failed (identical pre-existing, environment-specific failures already established as this session's baseline: `localStorage` unavailable in the Node test runner, one timing-sensitive test).
- `V-5` **Not executed**: a live click-through in the actual desktop app. Attempted via the Vite dev-server preview; the app's bootstrap gate requires a reachable local-runner backend (`127.0.0.1:4317/supabase-config`) which is unavailable in this sandboxed environment (`net::ERR_CONNECTION_REFUSED`), so it never reaches the main chat UI where `FlowAwaitingUserCard` would render. Confirmed instead: every new/changed module (including `FlowAwaitingUserCard.tsx`) loaded over the dev server with zero console/network errors — rules out an import/syntax crash, but does not confirm the rendered/interactive UI. A live re-check with a real backend is still pending.

## 9. Regression Guard

- tests: the 6 new Go tests (§8 `V-1`) lock in the escalate/cap-reached settlement and the Continue resume semantics; the 3 new desktop tests (§8 `V-3`) lock in the status derivation and the `continueFlow` action.
- alerts: none.
- audit checks: recorded in `change-audit/CA-233-escalate-awaiting-user-actionable.md`.

## 10. Follow-Up Document Updates

- Updated as part of this fix: `SD-19-Agent-Flow-Engine.md` §8 (`F-3`) — added the BUG-231 settlement contract (waiting step status, `BlockReason`, non-`running` client status, hub self-routing on resume). `CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md` `P-4` — noted that this fix makes the `blocked` pause deterministic at the engine layer rather than solely relying on the skill calling `ask_user`, and that `ExtendMax` is retired as a hard reject.
- Not updated: `SS-05`/`SD-06` (no model/provider-resolution rule changed by this fix).

## 11. Definition of Done

Each item is a concrete, independently-verifiable outcome. `[x]` = done, `[ ]` = not started/deferred.

### Runner (Go — `apps/local-runner`)

- `[x]` **DOD-R1 — Distinguish the two block reasons.** `AgentLoopState` carries an explicit `BlockReason` (`"cap"` | `"escalate"`), set whenever `Status` becomes `blocked`. UI/logic never has to parse `GateReason` prose to know why the loop paused.
- `[x]` **DOD-R2 — Escalate settles the hub node to a waiting (not RUNNING, not FAILED) state.** After `applyFlowControl("escalate")` on a flow-engine-driven run, the hub inline (`synthesis`) node is `WAITING_USER_APPROVAL` on the step timeline; the loop stays `blocked`/`awaiting_user` (never auto-`done`/`failed`).
- `[x]` **DOD-R3 — Cap-reached settles the hub node the same way.** When the `continue` case blocks on `Round >= cap`, the hub node is likewise `WAITING_USER_APPROVAL` (today it can be left `RUNNING`), with `BlockReason="cap"`.
- `[x]` **DOD-R4 — New unified resume action.** `POST /client/workflow-runs/{runId}/agent-loop/continue` with body `{"feedback": "..."}` (feedback optional): injects feedback into the hub context, moves the loop `blocked`→`running` (auto-raising the cap by `ExtendBy` only when `BlockReason="cap"`), transitions the hub node back to `RUNNING`, and re-invokes the hub synthesis turn. Returns an `AgentGraphSnapshot`. Idempotent: a second call while already running is a no-op.
- `[x]` **DOD-R5 — Hub re-decides and self-routes.** After a Continue resume, the re-run hub synthesis turn calls `submit_review_outcome` and routes itself: `changes_requested`→coder, `approved`→`done`, `blocked`→pause again (form re-appears). No hard-coded route in the resume action.
- `[x]` **DOD-R6 — `ExtendMax` no longer blocks.** `extendCap` (and the auto-extend inside DOD-R4) no longer reject on `ExtendCount >= ExtendMax`. `Cap` (auto-loop bound) is unchanged. `ExtendCount` still increments for display/telemetry.

### Desktop (`apps/desktop-flowpilot` + `packages/flowpilot-client-core`)

- `[x]` **DOD-D1 — Blocked no longer reads as "running".** `deriveOrchestrationRunStatus` maps loop `blocked`/`awaiting_user` to a non-`running` awaiting status, so the composer is NOT gated as "Waiting for the current turn…".
- `[x]` **DOD-D2 — Contract + client method.** `RunnerClient.continueFlow(parentRunId, feedback)` exists and is implemented in `HttpWsRunnerClient` (POST to the new endpoint) and `MockRunnerClient`. `AgentLoopState` type gains `blockReason`.
- `[x]` **DOD-D3 — Store action.** `continueFlow(feedback)` store action calls the client and applies the returned snapshot, mirroring `extendCap`/`stopAgentLoop`.
- `[x]` **DOD-D4 — Inline Continue/Stop form in main chat.** When the run's loop is `blocked`/`awaiting_user`, the main chat renders a form (modeled on `GateBlockModal`/`QuestionCard`): shows the `gateReason`, a feedback textarea, a **Continue** button (calls `continueFlow`), and a **Stop** button (calls the existing `stopAgentLoop`). It disappears once the loop leaves `blocked`. Implemented as `FlowAwaitingUserCard.tsx`. *Caveat*: code-complete and typechecked/built clean, but NOT visually confirmed in a running app — see §8 `V-5`.
- `[x]` **DOD-D5 — Step timeline shows "waiting on you".** The `synthesis` step row rendered from `WAITING_USER_APPROVAL` displays a distinct waiting indicator (not the RUNNING spinner) in `FlowStepTimeline`/sidebar. Turned out to already be fully implemented (`visualState` already maps `WAITING_USER_APPROVAL` to a dedicated `"approval"` state) — no code change needed, just confirmed by reading.
- `[x]` **DOD-D6 — Board consistency.** The OrchestrationBoard blocked banner uses the same `continueFlow` path (or is folded into it); the standalone "Extend cap +2" button is either removed or made an unbounded convenience alias, so there are not two inconsistent extension paths. Folded into `continueFlow`; the standalone button was removed.

### Cross-cutting

- `[x]` **DOD-T1 — Runner tests.** Escalate → hub `WAITING_USER_APPROVAL` + loop `blocked` + `BlockReason="escalate"`; cap-reached → same with `BlockReason="cap"`; `continueFlow` resumes (running, hub RUNNING, synthesis turn re-invoked, feedback in context) and auto-extends only for `cap`; `extendCap` no longer errors past `ExtendMax`.
- `[x]` **DOD-T2 — Desktop tests.** `deriveOrchestrationRunStatus` returns a non-`running` status for loop `blocked`; `continueFlow` store action applies the snapshot. (Component-level test for the form's blocked-only visibility was not added — this repo's test harness doesn't do React component rendering tests; covered instead by the store-level `deriveOrchestrationRunStatus`/`continueFlow` tests plus manual code review of `FlowAwaitingUserCard`'s `if (!loopState || loopState.status !== "blocked") return null;` guard.)
- `[x]` **DOD-T3 — Full regression green.** `go build`/`go vet`/`go test ./internal/runner/...` at the established baseline (no new failures); desktop `typecheck`/`build`/unit tests pass.
- `[x]` **DOD-DOC — Docs.** `SD-19`/`CP-36` updated for the awaiting-user settlement contract; `change-audit/CA-233-escalate-awaiting-user-actionable.md` written; this doc moved to `done/` with a Validation section.
- `[ ]` **DOD-Q3 — (Separate) note-delivery trigger.** Reproduced with a live runner log and either fixed or downgraded to a documented model-reliability note. Deferred — does not block DOD-R/D above, which are all complete.

## 12. Code Change Plan (per DoD item)

### DOD-R1 — `BlockReason` on `AgentLoopState`
- `provider_event.go:179` — add `BlockReason string \`json:"blockReason,omitempty"\`` to `AgentLoopState`.
- `interactive_service.go` `applyFlowControl`:
  - `continue` case cap-reached branch (`:532-535`): set `st.BlockReason = "cap"` alongside `st.Status="blocked"`.
  - `escalate` case (`:569-577`): set `st.BlockReason = "escalate"`.
  - On any resume/run transition (extendCap, resumeFlowWithFeedback), clear `st.BlockReason = ""` when `Status` returns to `running`.

### DOD-R2 / DOD-R3 — settle hub node to `WAITING_USER_APPROVAL`
- Add helper `setFlowStepAwaitingUser(ctx, parentRunID)` in `flow_step_runtime.go` (mirrors `setFlowStepStatus` but targets `hubInlineNodeID(activeFlowNodesFor(runID))` → `StepStatusWaitingUserApr`; no-op if not flow-engine-driven or no hub node).
- Call it from `applyFlowControl` `escalate` case and the `continue` cap-reached branch, right after the `mutateLoop` (both currently gated by `isFlowEngineDriven`). Reuses the existing `hubInlineNodeID` helper used by the CA-226 fallback (`interactive_service.go:2451`).

### DOD-R4 / DOD-R5 — resume action `resumeFlowWithFeedback` + endpoint
- `interactive_service.go` — new `func (s *InteractiveService) resumeFlowWithFeedback(parentRunID, feedback string) (AgentGraphSnapshot, error)`:
  1. If `feedback != ""`, `s.appendPendingAgentContext(parentRunID, "[flow-engine] User guidance: "+feedback)`.
  2. `mutateLoop`: read `BlockReason`; if `"cap"`, `st.Cap += ExtendBy` (default 2) and `st.ExtendCount++`; set `st.Status="running"`, `st.GateReason=""`, `st.BlockReason=""`. No-op (return snapshot) if status was not `blocked`.
  3. Transition hub node `WAITING_USER_APPROVAL`→`RUNNING` (`setFlowStepStatus(..., StepStatusRunning)`).
  4. `go s.maybeAutoReinvokeHub(parentRunID)` (drains pendingAgentContext incl. the guidance into the synthesis turn).
  5. `emitAgentGraph` + `persistParentSession`.
- `interactive_handlers.go` — new `handleContinueFlow` (body `{"feedback":string}`), registered at `RegisterInteractiveRoutes` (`:61` area) as `POST /client/workflow-runs/{runId}/agent-loop/continue`. Mirrors `handleExtendCap`'s run-exists check + snapshot response.

### DOD-R6 — retire `ExtendMax` enforcement
- `interactive_service.go` `extendCap` (`:599-`): remove the `if st.ExtendCount >= defaultExtendMax { extendErr = ... }` early-return; keep `st.ExtendCount++`. (Or gate it behind a debug flag — default off.)
- Update `TestExtendCap*` (`interactive_service_test.go:1616-1621`) that assert the error past `ExtendMax` → assert it now succeeds and keeps raising the cap.

### DOD-D1 — status derivation
- `store.ts` `deriveOrchestrationRunStatus` (`:2245-2249`): split the `blocked` case out of the `running` group → return a non-`running` awaiting status (reuse `"waiting_question"` semantics, or add a dedicated `"awaiting_user"` `RunStatus`). Ensure `ChatInput`'s `blocked` gate (`ChatInput.tsx:489`) does NOT treat it as busy.

### DOD-D2 — contract + client
- `contract.ts`: add `continueFlow?(parentRunId: string, feedback: string): Promise<AgentGraphSnapshot>` to `RunnerClient`; add `blockReason?: string` to `AgentLoopState`.
- `HttpWsRunnerClient.ts` (`:197-202` area): `continueFlow(id, feedback) { return this.postJSON(.../agent-loop/continue, { feedback }); }`.
- `MockRunnerClient.ts`: add a `continueFlow` that flips its mock loopState back to `running`.

### DOD-D3 — store action
- `store.ts` (`:529-534` area, beside `extendCap`): `async continueFlow(feedback) { ... parentRunId ... if (client.continueFlow) set(applyAgentGraphSnapshot(await client.continueFlow(parentRunId, feedback))); }`. Add to the store type/interface.

### DOD-D4 — inline Continue/Stop form
- New component (e.g. `FlowAwaitingUserCard.tsx`) modeled on `ChatWorkspace.tsx` `GateBlockModal` (`:407`): reads `agentGraphSnapshot.loopState`; renders only when `status==="blocked"`. Shows `gateReason`, a textarea (local state), **Continue** (`void continueFlow(text)`), **Stop** (`void stopAgentLoop()`). Mount it in `ChatWorkspace` near `GateBlockModal` (`:648`) / above the composer.

### DOD-D5 — step-timeline waiting indicator
- `FlowStepTimeline.tsx` / `FlowTimelineSidebar.tsx`: map `WAITING_USER_APPROVAL` to a distinct badge/icon ("⏸ waiting for you") rather than the RUNNING spinner (a WAITING_USER_APPROVAL branch may already exist for the approval flow — reuse it).

### DOD-D6 — board consistency
- `OrchestrationBoard.tsx` (`:127-130`): point the blocked banner's action at `continueFlow` (with an optional feedback field) or hide the standalone "Extend cap +2" when the new form covers it. Keep one extension path.

### DOD-T1/T2 — tests
- Go: extend `interactive_service_e2e_test.go` (`TestE2EReviewLoopEscalatePath`, `TestE2EReviewLoopCapHitBlocked`) to also assert hub node `WAITING_USER_APPROVAL` + `BlockReason`; add `TestResumeFlowWithFeedbackResumesAndReinvokesHub` and `TestResumeFlowWithFeedbackAutoExtendsOnlyForCap`; update `extendCap` past-`ExtendMax` test.
- Desktop: `store.test.ts` — `deriveOrchestrationRunStatus` blocked→non-running; `continueFlow` applies snapshot. Component test for the form's blocked-only visibility if the harness supports it.

## 13. Follow-Up Fixes From Live Testing

Live testing of the feature above (real runner, mixed-verdict review) surfaced 4 further issues. Two were regressions introduced by this bug's own `FlowAwaitingUserCard`/`continueFlow` code and are fixed here as amendments; the other two are pre-existing, unrelated defects and are tracked (and now also fixed) in a separate doc, [BUG-233](BUG-233-Flow-Timeline-Staleness-And-Blocked-Card-Diagnostic-Content.md).

- **Fixed — form too narrow.** `FlowAwaitingUserCard.tsx`'s feedback `<textarea>` sat as a sibling of `.other-row` (the button row), not inside a flex row, so it never picked up `.text-input`'s `flex: 1` expansion and rendered at the browser default width. Added a dedicated `.flow-awaiting-user-feedback` CSS rule (`width: 100%`, `box-sizing: border-box`, block display) in `styles.css`, and raised `rows` from 2 to 4.
- **Fixed — Continue while a child agent is focused leaked the resumed turn into both transcripts.** `continueFlow` called `client.continueFlow` unconditionally, without the child-focus guard `sendPrompt`/`stop` already have. Added the same `activeAgentRunId !== parentRunId` check to `store.ts`'s `continueFlow`, calling `backToMainRun()` first so the resumed hub turn streams only into the main transcript.
- **Fixed in BUG-233** — step timeline shows stale "all done" while a reviewer child is still running (turned out to be a server-side synchronization bug, not the client-side refresh-channel race originally suspected).
- **Fixed in BUG-233** — the blocked card sometimes shows the CA-226 internal diagnostic Summary verbatim instead of the reviewers' actual findings when the hub skips `submit_review_outcome`.

Regression: added `store.test.ts` — `"continueFlow returns to the main run before resuming when a child agent is focused (BUG-231 follow-up)"`.
