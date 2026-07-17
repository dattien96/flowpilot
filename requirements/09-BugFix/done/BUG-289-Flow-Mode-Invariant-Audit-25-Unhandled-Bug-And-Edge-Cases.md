# BUG-289: Flow Mode Invariant Audit — 25 Un-Handled Bug/Edge Cases (5 Silent-Hang Class)

## Metadata

- Document ID: `BUG-289`
- Title: `Flow Mode Invariant Audit — 25 Un-Handled Bug/Edge Cases (5 Silent-Hang Class)`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-17`
- Last Updated: `2026-07-17`
- Parent Documents: [Task-238: Flow-Mode Node/Edge State-Machine Hardening Audit](../../08-Task/done/Task-238-Flow-Mode-Node-Edge-Behavior-State-Machine-Hardening-Audit.md) (the `I-1..I-17` invariant catalog this audit re-verifies), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [SD-21: Change Contract And Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md), [SD-24: Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md)
- Child Documents: `None`
- Related Documents: [BUG-288](../inprogress/BUG-288-Flow-Mode-Three-Tier-Gate-And-Change-Contract-Reentry-Gaps.md) (the multi-round epic this audit follows up; several findings are gaps adjacent to R11–R20 fixes), [BUG-231](../done/BUG-231-Escalate-Awaiting-User-State-Not-Actionable-Flow-Hangs.md) (I-6/I-16 prior hang), [BUG-284](../done/BUG-284-Hub-Notify-Deferred-Reinvoke-Retried-With-Wrong-Generic-Prompt.md) / [BUG-285](../done/BUG-285-Hub-Notify-Reinvoke-Dropped-When-Loop-Blocked-Mid-Defer.md) (hub.notify reinvoke re-arm), [BUG-176](../done/BUG-176-Cohort-Reviewer-Can-Terminate-Flow-Before-Join-Barrier.md) / [BUG-234](../done/BUG-234-Loop-Back-Lifecycle-Runaway-Advance-And-Cohort-Node-Settlement.md) (cohort barrier), [BUG-251](../done/BUG-251-Restarted-Child-Agent-Shows-Permanently-Stale-Running-Status.md) (stale running badge), [CP-51](../../07-Coding-Plan/inprogress/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md) (dispatch findings), [CP-43](../../07-Coding-Plan/inprogress/CP-43-Change-Contract-And-Canonical-Intent-Signature.md), [Task-251](../../08-Task/inprogress/Task-251-Gate-Checkpoint-Durability-And-Retry-Worker.md) (**2026-07-17 ownership decision**: `A3`/`F-7` — the settle-driver production-caller wiring — is owned and delivered once here, not duplicated in Task-251; Task-251 stays `in_progress` until `F-7` lands and closes both together). Evidence: multi-agent audit workflow runs `wmv0w7g1y.output` (round 1, 17 findings) and `wmbmajnn9.output` (round 2, 12 findings) — scratchpad task outputs.
- Replaces: `None`
- Tags: `agent-flow-engine, flow-mode, hang, gate, cohort, restore, dispatch, timeline, invariant-audit, regression, severity-critical, multi-finding`

## AI Quick View

### Summary

- A multi-agent audit (16 area finders + adversarial verification + hand-verification of the top 3) re-checked the whole Flow Mode engine against the `I-1..I-17` invariant catalog and surfaced **25 deduped un-handled bug/edge cases** that survived the BUG-288 epic and Task 238–258.
- **5 are CRITICAL silent-hang cases** (`H1..H5`) — the loop ends up `running` (or `WAITING`) with no turn in flight, no gate/approval surfaced, no reprompt, and no user-visible reason. `H1`, `H2`, `H3` were **verified by hand against current code**; `H4`, `H5` are high-confidence finder findings.
- Recurring anti-pattern across nearly every hang: *a flag/lease/status is SET on the happy path, but a rare error / restart / race branch fails to CLEAR it or to leave an actionable state.* The existing `I-16` escape hatch (`checkAndBlockStalledMembers`) only covers **cohort members**, not the **hub/root** — which is why this whole class leaks.
- The remaining 20 findings are HIGH (state/correctness), MEDIUM (durability/concurrency), and LOW (timeline honesty / mis-authored-pack robustness). Two findings (`M1` repromptAttempts, `H3` commit-block) were found **independently by two separate audit rounds**, and `H3` was also confirmed by hand — high confidence.

### Current Ask

- **Done (2026-07-17):** point fixes for hang/correctness findings (`F-0`..`F-6`, `F-8`..`F-12`, and **A2** half of `F-7`) landed; package build + focused `TestBug289_*` green.
- **A3 residual closed via Task-251 (same day):** production `EvaluateGate` + boot/live `scheduleSettleDrive` + `RetrySettleWithBackoff` land under Task-251; nil-hook still refused; no run state still fail-closed. See [Task-251](../../08-Task/done/Task-251-Gate-Checkpoint-Durability-And-Retry-Worker.md).
- Task-255 closed same pass (B8a–e, model suite, CI race).

### Key Decisions

- `V-1` A finding is only listed when it cites a concrete `file:line` + a reachable `failure_scenario`; every candidate was checked against existing guards (`why_not_already_handled`) and dropped if a guard fully covered it.
- `V-2` `H1`, `H2`, `H3` are marked **hand-verified**: the set/clear/emit sites and call graph were read directly in this session, not just finder-reported.
- `V-3` One consolidated doc (not 25 files) per owner instruction ("1 bug cho all issue"); precedent = BUG-288 multi-round epic. Each finding carries a stable `H*/A*/M*/L*` ID for later split-out if fixed separately.

### Constraints

- Production point fixes for all 24 findings + `F-0` landed 2026-07-17. Package build green; focused runner tests green. Remaining gap is §8 acceptance tests + CI on Unix.
- GitNexus impact analysis MUST be run before editing any of the cited symbols (repo policy); several sit on hot paths (`startTurn`, `runTurn`, `runFlowGateAtEpoch`, `applyFlowControl`) with HIGH blast radius.
- Fixes touch the same hot functions BUG-288 hardened — sequence carefully to avoid re-introducing R11–R20 regressions.

### Open Questions

- `Q-1` Should `F-0` (hub watchdog) reuse the `member_stalled` block/`GateReason` surface, or introduce a distinct `hub_stalled` reason so the card copy differs?
- `Q-2` For `H3`/`SaveHead`, should a corrupt canonical head **self-heal** (rebuild from contracts NDJSON) or only fail-closed-with-surface? Self-heal is more robust but risks masking a real Drive-sync conflict.
- `Q-3` **Resolved (2026-07-17):** `A3` is owned and delivered **here** (`F-7`), not duplicated in Task-251 — Task-251's own completion notes already identified the same "`SettleDriver` has 0 production callers" gap as its deferred T-1/T-3 work; rather than fix it twice under two documents, Task-251 now explicitly hands that work off to this bug's `F-7` and stays `in_progress` until it lands (see [Task-251](../../08-Task/inprogress/Task-251-Gate-Checkpoint-Durability-And-Retry-Worker.md) §8). `A2` overlaps Task-250 (closed 2026-07-17): Task-250 waived building real provider-cancel/reconcile (T-4) because no adapter exposes the capability — that waiver covered *correctness* (no duplicate/no loss), but did not address the *durability* consequence `A2` raises here (a Stop-then-crash record stuck non-terminal forever blocks the 90-day prune same as `A3`). `A2` stays in this bug's scope; `F-7`'s production-caller wiring should resolve both `A2`'s and `A3`'s prune-blocking symptom via the same settle/recovery path, even though the underlying causes (no adapter reconcile vs. no driver caller) differ.

### Source Refs

- Round 1 audit output (17 findings, 1 adversarially confirmed): `.../tasks/wmv0w7g1y.output`.
- Round 2 lean audit output (12 findings): `.../tasks/wmbmajnn9.output`.
- Invariant catalog: `Task-238` §4.0 (`I-1..I-17`).
- Hand-verification anchors (this session): `interactive_service.go:1706/2131/6047/849` (H1), `cohort_stall.go:215` + `interactive_service.go:4662` + `appendCohortResult` call sites (H2), `gate_hook.go:137/178` vs `305/722` + `changecontract/head.go:99` (H3).

## 1. Issue Summary

Flow Mode ("agent-flow-engine") has been hardened across ~20 rounds (BUG-288) and Task 238–258, yet a systematic re-audit against its own invariant catalog (`I-1..I-17`) still finds 25 un-handled defects. The owner's foremost requirement is that **the flow must never hang** — reach a state that is neither terminal nor actionable, with no user-visible reason. Five findings violate exactly that. The rest degrade state correctness, durable-store integrity, timeline honesty, or robustness against mis-authored flow packs. This document catalogs all of them with reproductions and proposed fixes; it does not change code.

## 2. Parent Links

- impacted coding plan: [CP-51 Durable Turn Dispatch](../../07-Coding-Plan/inprogress/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md) (`A2`,`A3`,`M2`,`M3`,`L1`), [CP-43 Change Contract](../../07-Coding-Plan/inprogress/CP-43-Change-Contract-And-Canonical-Intent-Signature.md) (`H3`,`M6`,`SaveHead`), [CP-42 Flow Pack / Generic Node Behavior](../../07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) (`A5`,`A6`,`L2`,`L5`).
- impacted tech design: [SD-19 Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) (turn/cohort/synthesis/edge/restore), [SD-20 Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md) (`H3`,`M6`,`L3`), [SD-21 Change Contract](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md) (`H3`,`SaveHead`), [SD-24 Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md) (`A1`,`A2`,`A3`,`M2`,`M3`,`L1`).
- impacted system spec: none directly; `I-1..I-17` (Task-238) is the operative contract these findings violate.

## 3. Environment and Reproduction

- environment: `apps/local-runner` (Go), Flow Mode / Chat "bug" sub-mode, review-loop / context-coding-review-synthesis / rag-harness flow shapes; V2 dispatch active for Claude/Codex/Grok.
- reproduction: per-finding `failure_scenario` in §6 and the two evidence outputs. Hang cases generally need one of: a transient store/IO error, a provider-account rotation mid-loop, a server restart across a TTL/gate window, or a mixed-outcome / hub-less flow shape.
- frequency: `H1`/`H3`/`H4` are condition-triggered (not every run) but reachable in normal operation (e.g. account swap after usage-limit, corrupt Drive-synced head, restart during approval). `H2` triggers whenever a cohort member's first turn fails pre-flight. `M1` is deterministic after 2 lifetime reprompts.

## 4. Expected vs Actual

- expected: every park/error/restart/race path leaves an ACTIONABLE state (`I-6`), every barrier has an escape hatch (`I-16`), terminal state is monotonic (`I-3`), and the timeline reflects true state (`I-1`/`I-2`).
- actual: 5 paths dead-end the loop with no actionable surface; several paths regress terminal state, leak durable records, or mis-render the timeline. Details in §6.

## 5. Impact

- users affected: anyone running Flow Mode / orchestrated Chat "bug" mode, especially multi-provider review cohorts, long-running flows across restarts, and flows on repos with `.flowpilot/canonical` heads (esp. Drive-synced).
- workflows affected: review-loop synthesis, cohort join, three-tier gate, restore/resume, hub.notify, validate/audit, CP-51 dispatch recovery/settle, per-project dispatch store.
- severity: 5 CRITICAL (silent hang / lost work), 7 HIGH, 7 MEDIUM, 5 LOW.

## 6. Root Cause

Hypothesis-vs-confirmed status is marked per finding. **Confirmed (hand-verified)**: `H1`, `H2`, `H3`. **Finder-diagnosed (not yet hand-verified)**: all others.

### 6.0-a Verification status (2026-07-17 follow-up pass)

All 24 itemized findings (`H1`–`H5`, `A1`–`A7`, `M1`–`M7`, `L1`–`L5`) have now been **hand-verified against current code** (not just finder-reported) — 24/24 confirmed, zero refutations. Notable confirmations from the second pass: `M2` (RAM mutated + intent cleared before `commitLine`/fsync, `dispatch_store_memory.go:548-568`), `M3` (`forRun` scans `st.records` under the wrong mutex, `dispatch_store_open.go:143`), `M6` (`oracle.Failed`/`nowFailed` is unfiltered by `IsOverridden` — only `Regressed` is, `oracle.go:96/110/137`), `L1` (`Compact`'s "clears, effects, repairs" comment vs. a loop that only ever writes `clears`, `dispatch_store_local.go:261-272`), `L3` (`pendingGateBlock` assigned exactly once in the whole file — root only, `gate_hook.go:338`), `L4`/`L5` (both confirmed by direct comparison against their sibling "happy path" that DOES do the write). Key confirmations beyond the original write-up:

- `H4`: the already-expired-at-rehydrate branch (`interactive_service.go:2969-2982`) `continue`s without ever setting a live-card flag, so `rs.status` is left at `WaitingApproval` with nothing to reset it — matches finding exactly.
- `H5`: confirmed the second drain site (`resumeFlowWithFeedback`, `:1500-1502`) is gated by `if !wasBlocked { return }` — since the loop stays `"running"` (not `"blocked"`) in this scenario, this drain never engages either. Two drain sites total, both dead in this scenario.
- `A6`: confirmed `advanceHubDoneThroughEdge`'s hub.notify success path (`:4305-4315`) returns `handled=true` with no `lastFlowControlTurnID` stamp, and the bridge's one-decision guard (`:4239`, `flowControlSubmittedForTurn`) reads exactly that field — so a second same-turn call passes through untouched.
- `A3`: independently corroborated via GitNexus symbol graph — `SettleDriver` has exactly one caller, a test.

### 6.0 Cross-cutting root cause

A pervasive anti-pattern: an in-flight flag / lease / status / pending-slot is set on the happy path, but a rare branch (early-return `*apiErr`, store/IO error, `ctx` cancel, panic, server restart, TTL expiry, or a lost race) neither clears it nor leaves an actionable state. Because `I-16`'s runtime escape hatch (`checkAndBlockStalledMembers`, `cohort_stall.go`) only inspects **cohort members**, a wedged **hub/root** has no safety net → silent hang.

### 6.1 CRITICAL — Silent Hang (H1–H5)

- `H1` **(confirmed)** `reinvokeInFlight` leaks `true` when a scheduled hub-reinvoke's `startTurn` is rejected. `maybeAutoReinvokeHubWithNote`/`WithPrompt` set `parent.reinvokeInFlight = true` (`interactive_service.go:1706`/`:1804`) then `scheduleChildTurn` runs `go func(){ _, _ = s.startTurn(...) }` (`:2130-2132`) discarding the error. `reinvokeInFlight` is cleared ONLY at `startTurn:6047` (success) and `stopAgentLoop:849` (Stop). `startTurn` has error-returns BEFORE 6047 (`dispatch_prepare_failed` :6041, `provider_v2_disabled` :6036, `provider_account_changed`, `awaiting_user`, `gate_in_progress`). On any such reject the flag stays true forever; all later reinvokes short-circuit at the `:1661`/`:1759` guard and the `:1677` deferred re-arm requires `!reinvokeInFlight` so `pendingHubReinvoke` is never set. Hub never synthesizes. Escape = user Stop only. Violates `I-6`/`I-16`. **Fix (`F-1`): done** — `interactive_service.go` — in `scheduleChildTurn` (`:2130-2132`), capture `startTurn`'s returned error (currently discarded); on error, lock and clear `parent.reinvokeInFlight=false`, re-arming `pendingHubReinvoke`(+prompt) so a later completion drains it. **Review fix (2026-07-17):** also call `notifyTurnIdle(runID)` after re-arm (same pattern as H4 live expire) so the pending is actively drained/retried — re-arm alone only renamed the stuck flag while F-0 treated `pendingHubReinvoke` as busy forever; F-0 no longer counts undrained pending as busy; cap consecutive start fails at 3 to avoid tight-loop.
- `H2` **(confirmed)** Cohort member whose first turn fails pre-flight is never buffered and never stalls → barrier never joins. `spawnChildRun`'s async first turn (`interactive_service.go:4657-4666`) calls `signalChild(...,RunStatusFailed)` on `startTurn` error but never `appendCohortResult`. Production `appendCohortResult` sites are turn-completed/failed handlers (`:3500`,`:3815`), the stall sweep (`cohort_stall.go:347`), Stop (`:986`), and resume back-fill (`interactive_resume.go:1776`). A pre-flight failure hits none of them live. The stall sweep skips the member via the `else { continue }` at `cohort_stall.go:215-217` (not terminal, `last.IsZero()`, `!inFlight`), and `maybeScheduleStallCheck` (`:121-123`) arms no timer. `cohortComplete` stays false forever; only a restart back-fills it. Violates `I-11`/`I-16`. **Fix (`F-2`): done** — `interactive_service.go` — in `spawnChildRun`'s pre-flight error branch (`:4662-4665`), when `flowCohortId != ""` also call `appendCohortResult(parentRunID, cohortID, cohortEntry{Status:"failed", ...})` and re-check `cohortComplete`/hub reinvoke, mirroring the `EventTurnFailed` handler at `:3831-3851`.
- `H3` **(confirmed)** Head write/read failure turns a GREEN turn into a silent permanent gate block. On `len(violations)==0`, `commitChangeContract` → `updateCanonicalHead` → `changecontract.LoadHead` on a corrupt `.flowpilot/canonical/<feature>.json` (torn `SaveHead`, Drive-sync conflict copy, or concurrent same-feature writers) returns an error; `withGateEpochDurable` returns false and BOTH gate paths `return true` (block) WITHOUT emitting `EventFlowGateViolation` and WITHOUT `applyFlowControl(escalate)` — root gate `gate_hook.go:305-310` and child gate `:722-726`. This contrasts the sibling fail-closed sites `:137-146` (diff observe) and `:170-188` (baseline) which DO emit before blocking. Root cause: `SaveHead` (`changecontract/head.go:99`) is a plain non-atomic `os.WriteFile` (no temp+rename) and `LoadHead` (`:78-82`) hard-fails on corrupt JSON instead of tolerating/rebuilding. Retries re-read the same corrupt file. Violates `I-6`/`I-16`. **Fix (`F-3`): done** — `gate_hook.go` — make the two `commitChangeContract` block sites (`:305`, `:722`) emit `EventFlowGateViolation`(block) + `applyFlowControl(escalate)` like the sibling fail-closed sites; `changecontract/head.go` — rewrite `SaveHead` (`:99`) to write via temp-file + `os.Rename` (mirroring `flow_context_handoff.go:104-113`), and make `LoadHead` (`:78-82`) tolerate/rebuild a corrupt file instead of hard-failing.
- `H4` **(confirmed)** Rehydrated approval/question whose TTL elapses is marked `expired` but the run is never settled off `WAITING`. On restart `reconstructRunInternal` keeps `Status=WaitingApproval` and `rehydratePendingGatesLocked` (`interactive_service.go:2969` approval / `:3049` question) marks the card expired without flipping `rs.status`; `expireApproval` (`:4835`) / `expireQuestion` (`:4908`) clear `pendingApprovalID` but never touch `rs.status`. `SubmitApprovalDecision` then 409s. The live path only advanced because the blocked `RequestApproval` caller received `errApprovalExpired` — that goroutine is gone after restart. Run stuck `WAITING` forever. Violates `I-6`/`I-17`. **Fix (`F-4`): done** — `interactive_service.go` — in `expireApproval` (`:4835`) / `expireQuestion` (`:4908`) and the already-expired rehydrate branches (`:2969-2982`/`:3049-3057`), after marking the card expired also flip `rs.status` off `WaitingApproval`/`WaitingQuestion` to an actionable/terminal state.
- `H5` **(confirmed)** hub.notify reinvoke is drained while `turnInFlight` is still held for the post-turn gate, re-defers, and is never drained again. `dispatchHubNotifyNode` calls `maybeAutoReinvokeHubWithPrompt` from inside the finishing turn (`turnInFlight==true`) → defer branch (`:1759-1763`). The runTurn drain (`:5280`/`:5295`) dispatches `go maybeAutoReinvokeHubWithPrompt` but `willRunGate=completed=true` (`:5258`) keeps `turnInFlight` true through the (slow) gate; the goroutine wins the race, re-enters defer, re-arms `pendingHubReinvoke`. After the gate, `notifyTurnIdle` (`:1571`) flushes reprompt/resume intents but NOT `pendingHubReinvoke`; `resumeFlowWithFeedback` only fires when the loop is `blocked` (here it is `running`). Loop stays `running`, notify never sent, hang. Violates `I-6`/`I-7`. **Fix (`F-5`): done** — `interactive_service.go` — add a second drain of `pendingHubReinvoke`/`pendingHubReinvokePrompt` (mirroring `:5280-5301`) inside `notifyTurnIdle` (`:1571`) or right after the post-turn gate clears `turnInFlight`, not only in `runTurn`'s pre-gate tail.

### 6.2 HIGH (A1–A7)

- `A1` `linearizeSendStarted` returning false on a NON-Stop store/CAS error aborts `runTurn` (`interactive_service.go:5122-5130`) without `finishTurn`, leaving the step RUNNING and the UI turn stream with no terminal event. False is returned for store errors too (`dispatch_live.go:226`, `:272`), not only Stop. (`I-8`, hang-adjacent.) **Fix (`F-6`): done** — `dispatch_live.go` — give the non-Stop error returns (`:226`, `:258`, `:272`) a distinguishable signal from the Stop-caused ones (typed error/extra bool); `interactive_service.go` (`:5122-5130`) — on that signal, emit a terminal turn event and settle the RUNNING step instead of only `notifyTurnIdle`+return.
- `A2` Stop-then-crash on a `send_started`/`provider_accepted` dispatch leaves the record non-terminal forever: `reconcileOne` gets `RecoveryCancelRequired` and only `log.Printf`s (`dispatch_recovery.go:79-90`); `ListAttention` never surfaces it; every boot re-abandons it; blocks session prune. (`I-16`; overlaps CP-51 Task-250.) **Fix (`F-7`): done** — `dispatch_store_memory.go` — extend `ListAttention` (`:1291`) to also surface `DispatchSendStarted`/`DispatchProviderAccepted` records with `CancelRequested=true`; `dispatch_recovery.go` — in `reconcileOne`'s `RecoveryCancelRequired` branch (`:85-89`), stamp a durable marker (e.g. a `RepairRecord`) instead of only logging.
- `A3` `SettleOwed` records are created on every flow turn (`commitTerminal` → `SettlePending`) but `SettleDriver`/`CASAdvanceSettle` had **no production caller** (`dispatch_settle.go:21`), so `HasNonTerminal` stays true → 90-day prune blocked, `dispatch.ndjson` leaks. (overlaps CP-51 Task-251.) **Fix (`F-7`): partial — residual fail-closed (2026-07-17 review)** — first land of a boot `DriveSettle` with `SettleDriver{Store only}` was **unsafe**: nil `EvaluateGate` → `planNext` default `allow=true` → silent wrong finalize without `resumePendingFlowGate`. Corrected to: (1) boot **inventory only** (`inventoryPendingSettlesOnBoot`, no auto-advance); (2) `planNext` **errors** if `EvaluateGate == nil` at `settle_pending`; (3) `ListAttention` kind `settle_pending` for operator visibility. **Does not** unblock prune by fake-passing gates. Full settle (T-1 wire real EvaluateGate + resumePendingFlowGate into driver, T-2 real effects, T-3 RetrySettleWithBackoff caller, T-4 remove legacy 3-persist) remains **Task-251** — still `in_progress`.
- `A4` Validate-retry counter is not rebuilt on resume: `reconstructRunInternal` restores `planContextPackage` but leaves `rs.flowValidationRetryState==nil` (`flow_validate_audit_dispatch.go:395`), so `RetryAttempt` resets to 0 on every restart → max-retry cap not durable, coder re-armed past the cap. (`I-17`.) **Fix (`F-8`): done** — `interactive_resume.go` — in `reconstructRunInternal`, read back the durable `FlowValidationRetryState` (persisted via `EventFlowValidationRetry`, `provider_event.go:176`) the same way `planContextPackage` is restored, and set `rs.flowValidationRetryState` from it.
- `A5` Continue (`resumeFlowWithFeedback`) never re-runs an escalated inline validate/audit node in a hub-less (rag-harness) flow: `hubInlineNodeID`/`activeHubNodeID` are `""`, so it reinvokes a nonexistent hub instead of re-entering the escalated node (`interactive_service.go:1552`) → deterministic verify/audit gate bypassed on resume. (`I-6`/`I-9`.) **Fix (`F-9`): done** — `interactive_service.go` — in `resumeFlowWithFeedback` (`:1552`), before the unconditional hub-reinvoke fallback, check for a hub-less flow escalated from an inline node (needs a small new field remembering which node escalated) and re-dispatch that node directly (`runValidateNode`/`runAuditNode`).
- `A6` `advanceHubDoneThroughEdge` never stamps `lastFlowControlTurnID` (`interactive_service.go:4242`), so a second `flow_control` call in the same hub turn bypasses the `I-5` one-decision guard and can settle the flow past a `hub.notify` node a turn early (notification dropped). (`I-5`/`I-8`.) **Fix (`F-9`): done** — `interactive_service.go` — in `advanceHubDoneThroughEdge`'s hub.notify success branch (`:4305-4315`), stamp `rs.lastFlowControlTurnID = rs.currentTurnID` (same as `applyFlowControl` at `:1175`) before returning `handled=true`.
- `A7` Reconstructed child surfaces un-normalized `agentStatus` (`interactive_resume.go:820`): `rs.status` is normalized but `agentStatus` is not, and the live `listAgentRunSummaries` branch reads it raw → permanent stale "running"/"spawned" badge (BUG-251 regression on the live-reconstruct path). (`I-13`/`I-17`.) **Fix (`F-10`): done** — `interactive_resume.go` — in `reconstructRunInternal` (`:820`), normalize `agentStatus` the same way `status` is (via `normalizeResumedFlowStatus` or a parallel helper), instead of assigning `st.AgentStatus` verbatim.

### 6.3 MEDIUM (M1–M7)

- `M1` **(found by both rounds)** `repromptAttempts` is never reset and round-trips through resume (`gate_hook.go:369`/`:784`, persisted `:2644`, restored `interactive_resume.go:860`), turning the documented per-turn cap (`maxFlowGateReprompts=2`) into a per-run-lifetime budget — worse after restart. (`I-6` degraded.) **Fix (`F-8`): done** — `interactive_service.go` — in `startTurn`, add `rs.repromptAttempts = 0` at the start of every new turn (near `rs.turnCount++`, `~:6090`).
- `M2` Local NDJSON store mutates the in-memory record and clears the intent BEFORE `commitLine` fsync (`dispatch_store_memory.go:564`); on fsync failure RAM shows terminal while disk does not → double-run / false-complete on restart, contradicting the store's own disk-before-RAM header claim. (`I-13`.) **Fix (`F-11`): done** — `dispatch_store_memory.go` — in `commitTerminal` (`:540-571`, and sibling mutation funcs), call `s.commitLine(...)` BEFORE mutating `r.State`/`SettlePhase`/`clearIntentLocked`, so a commit failure leaves RAM untouched.
- `M3` `multiProjectDispatchStore.forRun`/`openProjectLocked` iterate `st.records` holding only `m.mu`, not the embedded `memoryDispatchStore.mu` (`dispatch_store_open.go:143`) → concurrent CAS write races the read → Go fatal "concurrent map iteration and map write" crashes the whole runner. **Fix (`F-11`): done** — `dispatch_store_open.go` — in `forRun` (`:137-150`), take the target `st`'s own lock (add a locked-iterate accessor on `memoryDispatchStore`) before scanning `st.records`, instead of relying on `m.mu` alone.
- `M4` `skipped_no_command` retry state is frozen in memory (`flow_validate_audit_dispatch.go:402`): after the first "no test command" escalate, a newly-configured command is loaded but discarded by the reset guard, so validate re-escalates "no command" forever within the process. **Fix (`F-8`): done** — `flow_validate_audit_dispatch.go` — broaden the reset guard at `:402` to also rebuild `state` when `state.Status=="skipped_no_command"` and the freshly-loaded `command` is now non-empty.
- `M5` `reconcileChildRunsOnFlowDone` loop 2 (`flow_step_runtime.go:487-495`) is not status-filtered and flips a FAILED cohort member's panel summary to Completed → masks a real failure (inverse of BUG-257). (`I-3`/`I-11`.) **Fix (`F-10`): done** — `flow_step_runtime.go` — apply loop 1's status filter (`:474-478`) to loop 2 (`:487-495`): skip any child whose run status is `Failed` instead of unconditionally promoting to `Completed`.
- `M6` r-tests (`tests_failed` always-block) ignores human-agreed test overrides: the `failedTests` path (`gate_hook.go:218-224`) copies raw `oracle.Failed` with no `IsOverridden` check, so an overridden regressed test still blocks; coarse-mode `suite_failed` cannot be overridden at all. **Fix (`F-12`): done** — `flowgate/oracle.go` — filter `nowFailed` by `IsOverridden` into `OracleResult.Failed` (`:137`) the same way `regressed` is filtered (`:96`/`:110`); or apply `IsOverridden` directly in `gate_hook.go`'s `failedTests` construction (`:218-224`).
- `M7` hub.notify reinvoke prompt is dropped in the `reinvokeInFlight && !turnInFlight` window: the re-arm predicate (`interactive_service.go:1760`) is `turnInFlight && !reinvokeInFlight` and does not cover the scheduled-but-not-started window, so the custom notify prompt is lost (notify step swept DONE, message never sent). (`I-7`; sibling of `H1`.) **Fix (`F-1`): done** — `interactive_service.go` — broaden the re-arm condition at `:1760` to also cover `reinvokeInFlight && !turnInFlight` (stash `pendingHubReinvokePrompt` in that window too).

### 6.4 LOW (L1–L5)

- `L1` `Compact` (`dispatch_store_local.go:261`) preserves records/clears but never writes `effects`/`releases`/`repairs`/`resolutions`; after compact+restart an open repair/quarantine, release manifest, attach-dedup, and resolution idempotency are lost. (Latent: `Compact` has no production caller yet.) **Fix (`F-11`): done** — `dispatch_store_local.go` — add loops mirroring the existing `s.clears` loop (`:262-272`) for `s.effects`, `s.releases`/`s.repairs`, and resolution results, each writing its own log-line `Kind` before `f.Sync()` at `:273`.
- `L2` The synchronous inline-node advance chain (`flow_validate_audit_dispatch.go:660`, `runValidateNode`→`advanceToNextInlineOrDelegate`→`runValidateNode`/`runAuditNode`) has no visited-set/depth cap → a mis-authored forward-`done` cycle among inline nodes recurses until stack overflow. (Needs a hand-authored pack.) **Fix (`F-9`): done** — `flow_validate_audit_dispatch.go` — thread a `visited map[string]bool` (or depth counter) through `tryAdvanceFlowThroughInline`/`advanceToNextInlineOrDelegate`'s recursive calls (`~:640-676`); escalate instead of recursing on a repeat.
- `L3` Child-detected regression escalates the parent with only `result.Message` (`gate_hook.go:766`) and never populates `pendingGateBlock`/`GateOptions`, so the user loses the keep-test / suggest-requirement / custom decision card the identical root-side regression offers. (`I-12` degraded.) **Fix (`F-12`): done** — `gate_hook.go` — in `runChildArtifactOutputGateAtEpoch`'s block branch (`:762-772`), populate `pendingGateBlock`/`GateOptions` the same way the root path does at `:338-339`.
- `L4` Failed-member cohort join reinvokes synthesis without stamping the hub inline node RUNNING (`interactive_service.go:3850`), unlike the completed-join path (`:3575-3590`) → timeline shows the running synthesis node as PENDING until settle. (`I-2`; self-heals, transient.) **Fix (`F-10`): done** — `interactive_service.go` — mirror `:3587-3589`'s `setFlowStepStatusLocked(..., hubNodeID, StepStatusRunning)` guard into the failed-member join branch, before `go maybeAutoReinvokeHubWithNote(...)` at `:3850`.
- `L5` `tryAdvanceFlowFromNode` delegate→inline single-target early-return (`flow_executor.go:1003-1011`) returns before the `setFlowStepStatus(DONE)` at `:1053`, so the delegate source step stays RUNNING through the validate+audit phase until the final sweep. (`I-13`/BUG-174 class.) **Fix (`F-10`): done** — `flow_executor.go` — before the `:1007` early return, add the `setFlowStepStatus(ctx, parentRunID, completedNodeID, StepStatusDone)` call the delegate-cohort path makes at `:1052-1053`.

## 7. Fix Strategy

Proposed, prioritized. **2026-07-17:** all `F-0`..`F-12` landed — each item below marked **done** (full wording preserved).

- `F-0` **(systemic, highest leverage — covers H1/H3/H4/H5) done** Extend the `I-16` runtime invariant from cohort members to the **hub/root**: a periodic watchdog that asserts *loop `running`/`WAITING` ⟹ (turn in flight) OR (gate/approval surfaced) OR (reinvoke pending) OR (actionable park)*; on violation for `T` seconds, block with an actionable `hub_stalled`/`member_stalled`-style card (Retry / Stop). This is a defense-in-depth net for the entire hang class, including future regressions of the same shape. Landed: `hub_stall.go`.
- `F-1` **(H1, M7) done** After `scheduleChildTurn`, on `startTurn` error clear `parent.reinvokeInFlight` and re-arm `pendingHubReinvoke`(+prompt) — stop discarding the error at `interactive_service.go:2131`. Broaden the `maybeAutoReinvokeHubWithPrompt` defer predicate to also cover `reinvokeInFlight && !turnInFlight`. **Also** `notifyTurnIdle` active drain after re-arm (Claude review); F-0 does not treat undrained pending as forever-busy.
- `F-2` **(H2) done** In `spawnChildRun`'s pre-flight error path (`interactive_service.go:4662-4665`), for a cohort member also `appendCohortResult(FAILED)` (and re-check `cohortComplete` / hub reinvoke), not only `signalChild`.
- `F-3` **(H3) done** Make the two `commitChangeContract` block sites (`gate_hook.go:305`, `:722`) emit `EventFlowGateViolation`(block) AND `applyFlowControl(escalate)` like the sibling fail-closed sites; make `SaveHead` atomic (temp+`os.Rename`, mirroring `flow_context_handoff.go:104-113`); make `LoadHead` corrupt-tolerant or rebuild from contracts NDJSON. (See `Q-2`.)
- `F-4` **(H4) done** When the sole pending approval/question expires (live `expireApproval`/`expireQuestion` and the rehydrate path), flip `rs.status` off `WAITING` to an actionable/terminal state and re-drive or park with a surfaced reason.
- `F-5` **(H5) done** Add a post-gate second drain of `pendingHubReinvoke` (e.g. in `notifyTurnIdle` or after `turnInFlight` is cleared following the gate), so a reinvoke deferred during the gate window is not stranded.
- `F-6` **(A1) done** `linearizeSendStarted==false` for a non-Stop store error must emit a terminal turn event and settle the RUNNING step (distinguish Stop from store-error).
- `F-7` **(A2 done; A3 residual fail-closed)** Wire CP-51 recovery/settle remediation: **A2** — surface stopped+sent stranded records via `ListAttention`/`OpenRepair`. **A3** — do **not** blind-`DriveSettle` on boot (that auto-allowed gates); fail-closed inventory + attention until Task-251 lands real `EvaluateGate`/effects/retry. Reconcile with Task-250/251 (`Q-3`).
- `F-8` **(A4, M1, M4) done** Rebuild `flowValidationRetryState` on resume; reset `repromptAttempts=0` at turn start; re-derive `skipped_no_command` when a command becomes available.
- `F-9` **(A5, A6, L2) done** Re-enter an escalated inline node on Continue for hub-less flows; stamp `lastFlowControlTurnID` on the `advanceHubDoneThroughEdge` success path; add a visited-set/depth cap to the inline advance chain (and reject inline forward-cycles at pack load).
- `F-10` **(A7, M5, L4, L5) done** Normalize `agentStatus` on child reconstruct and in the live summary branch; status-filter `reconcileChildRunsOnFlowDone` loop 2; stamp the hub node RUNNING on the failed-member join; mark the delegate source DONE on the delegate→inline early-return.
- `F-11` **(M2, M3, L1) done** fsync before mutating RAM/clearing intent (or roll back on fsync error); take `memoryDispatchStore.mu` in `multiProjectDispatchStore` scans; persist `effects/releases/repairs/resolutions` in `Compact`.
- `F-12` **(M6, L3) done** Apply `IsOverridden` on the `failedTests`/r-tests path; populate `pendingGateBlock`/`GateOptions` on the child-regression escalate.

### 7.1 Implementation Order (13 phases, all 24 findings)

Grouped by shared file/hot-path (fix once, test once) rather than strict severity, per BUG-288's own lesson (don't touch the same hot function in parallel rounds). All 5 CRITICAL land by Phase 5. Run `gitnexus_impact` before editing each symbol; green the cluster's tests before moving to the next phase.

| Phase | Findings | File(s) | Note |
| --- | --- | --- | --- |
| 0 | `F-0` (systemic) | new code (`hub_stall.go`) | **done** — Watchdog first — new, low-risk, immediate blanket protection while later phases land |
| 1 | `H1`, `M7`, `H5`, `A6` | `interactive_service.go` (reinvoke/turn-completion neighborhood), `interactive_resume.go` (`notifyTurnIdle`) | **done** |
| 2 | `H2`, `L4` | `interactive_service.go` (failed-join branch), `cohort_stall.go` | **done** |
| 3 | `M1` | `interactive_service.go` (`startTurn`) | **done** |
| 4 | `H3`, `L3`, `M6` | `gate_hook.go`, `changecontract/head.go` | **done** |
| 5 | `H4` | `interactive_service.go` (approval/question expiry) | **done** |
| 6 | `A4`, `A7` | `interactive_resume.go` | **done** |
| 7 | `A5` | `interactive_service.go` (`resumeFlowWithFeedback`) | **done** — after Phase 6 |
| 8 | `A1` | `dispatch_live.go` + `runTurn` | **done** — after Phase 1 stable |
| 9 | `M2`, `M3`, `L1` | `dispatch_store_memory.go`, `dispatch_store_open.go`, `dispatch_store_local.go` | **done** — independent, any time |
| 10 | `A2`, `A3` | `dispatch_recovery.go`, `dispatch_settle.go` / boot settle in `dispatch_live.go` | **A2 done; A3 residual fail-closed** — Task-251 still owns real settle driver (T-1..T-4) |
| 11 | `M4`, `L2` | `flow_validate_audit_dispatch.go` | **done** |
| 12 | `L5`, `M5` | `flow_executor.go`, `flow_step_runtime.go` | **done** — lowest risk, can also go earlier |

## 8. Validation

Code landed; package build green; focused suites / `TestBug289_*` green. Remaining §8 acceptance tests still useful for CI:

- `V-1` (H1) Unit test: schedule a reinvoke, force `startTurn` to return `dispatch_prepare_failed`, assert `reinvokeInFlight==false` and a later reinvoke fires (or `pendingHubReinvoke` re-arms).
- `V-2` (H2) Unit test: 2-member cohort, member A `startTurn` fails pre-flight, member B completes → assert barrier joins (member A buffered FAILED) or an actionable stall is raised within `T`.
- `V-3` (H3) Unit test: green turn + corrupt `<feature>.json` → assert `EventFlowGateViolation` emitted AND parent escalated (not silent). Plus a `SaveHead` crash-atomicity test.
- `V-4` (H4) Restart test: WAITING run whose approval TTL elapsed → assert run settles to an actionable/terminal state, not a 409-locked WAITING.
- `V-5` (H5) Ordering test through real `finishTurn→gate→idle` (not the direct `maybeAutoReinvokeHubWithPrompt` shortcut `flow_hub_notify_test.go` uses): assert the notify reinvoke is drained after the gate.
- `V-6` (A*/M*/L*) Per-finding targeted tests as each fix lands; add a state×event regression case per invariant touched (Task-238 `RU-2` traceability).
- `V-7` `go test ./internal/runner/... ./internal/flowgate/... ./internal/changecontract/...` green after each batch; `go vet`/race detector for `M2`/`M3`.

## 9. Regression Guard

- tests: the state×event matrices owned by Task-239/240/241/242 (Task-238 `RU-2`) extended with the new cases in §8; a dedicated hub-watchdog test for `F-0`.
- alerts: consider a diag counter for `hub_stalled` fires and for dispatch records stuck non-terminal past N boots (`A2`/`A3`).
- audit checks: a lint/CI assertion that every `return true` (block) in `gate_hook.go` is preceded by a violation emit (guards `H3`-class regressions).

## 10. Follow-Up Document Updates

- upstream docs that must change when fixes land:
  - `Task-238` / SD-19: add the hub-level `I-16` extension (`F-0`) as an invariant (currently `I-16` is member-only) — this is a genuine contract addition, not just a bug delta.
  - SD-20: note the "every block path must surface" rule as normative (guards `H3`/`M6`/`L3`).
  - SD-21 / CP-43: `SaveHead` atomicity + `LoadHead` corrupt-tolerance as a durability requirement.
  - SD-24 / CP-51: **`Q-3` revised (2026-07-17 residual review)** — `A2` landed here; `A3` only got fail-closed inventory + attention (not a real settle). Task-251 **remains `in_progress`**: T-1 (`resumePendingFlowGate` into driver + real `EvaluateGate`), T-2 (durable effect projection), T-3 (`RetrySettleWithBackoff` production caller), T-4 (remove legacy 3-persist) are **not** closed by BUG-289. Do not mark Task-251/Task-255 done from this bug.
- notes left unchanged on purpose: the LOW findings (`L1`,`L2`) are latent (no production caller / needs mis-authored pack) — recorded but not urgent.
