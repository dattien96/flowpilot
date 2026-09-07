# BUG-362: escalate Retry loops same validate fail; Now: stuck on stale plan_synthesis WAITING

## Metadata

- Document ID: `BUG-362`
- Title: `escalate Retry loops same validate fail; Now: stuck on stale plan_synthesis WAITING`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-07`
- Last Updated: `2026-09-07`
- Feature Keys: `agent-flow-engine` (dominant), `cli-tui`
- Parent Documents: [Task-325](../../08-Task/done/Task-325-Conditional-Plan-Approval-Park.md)
- Child Documents: `none`
- Related Documents: [CA-749](../../../change-audit/CA-749-Task-325-Plan-Approval-Park.md), [CA-758](../../../change-audit/CA-758-Run-207435-No-Repark-After-Freeze-Done.md), [CA-740](../../../change-audit/CA-740-audit-missing-ca-retry-writer.md)
- Replaces: `none`
- Tags: `plan-approval, escalate, retry-loop, step-status, tui-now`

## AI Quick View

### Summary

- Live `run-210188` (2026-09-07, TUI Task Harness, `opencode-go/muse-spark-1.3-contributor`, gate-sandbox): after Approve on `plan_approval` → freeze `[v]` → implement/validate, `TestDivide` panics. Loop `blocked/escalate` with "Validation failed after the maximum number of retries".
- CA-758 held: no `plan_approval` re-park, freeze stayed DONE. Operator kept clicking Retry → same escalate card forever ("run again with old scope").
- Sidebar showed `plan_synthesis WAITING_USER_APPROVAL` + `validate WAITING_USER_APPROVAL` and `Now: plan_synthesis` while the live gate was validate escalate. Footer `step: plan_synthesis`.

### Current Ask

- Implement F-1 + F-2 + F-3 in one slice after this plan. Do not touch CA-758 re-park guard, CA-749 first park, or muse-spark F-2 hang.

### Key Decisions

- `D-1` Dominant `feature_key: agent-flow-engine`. TUI `Now:` is a symptom of leftover runner step status, not a standalone display bug (CA-758 already proved the runner is source of truth).
- `D-2` F-1 is the real fix: when freeze becomes DONE, `plan_synthesis` must not stay `WAITING_USER_APPROVAL`. First park (freeze not DONE) still stamps WAITING (CA-749).
- `D-3` F-2 is defense-in-depth in `activeStepName`: prefer the last RUNNING, else the last WAITING (YAML order = topology). Do not pick the first WAITING hub.
- `D-4` F-3 does **not** hide Retry (flakes still need it). Specialize copy + default highlight when GateReason is validate-exhausted (`Validation failed after the maximum number of retries`): Retry stays, cursor/default is Revise, copy says spec/test conflict will fail again on old scope.
- `D-5` R2 Case 1: no `providerKey` branch. Parameterize new tests Claude/Codex/Grok anyway so a future provider split trips.

### Constraints

- safe-fix-contract: additive tests only; no old-test edits; stop on old fail.
- Will-not-undo: CA-749 first churned park WAITING + `blocked/plan_approval`; CA-758 skip-repark after freeze DONE; CA-752 empty=approve; CA-757 verdict gate first; CA-741 park-cancel; CA-740 missing-CA Retry copy.
- Must not treat a first-time validate flake Retry as a spec conflict.
- Must not clear `plan_synthesis` WAITING while freeze is still pending (operator has not Approved yet).

### Open Questions

- `Q-1` Closed: Retry looping is escalate-by-design on old scope, not a hang. Fix is copy + default chip, not a timeout.
- `Q-2` Closed: `Now: plan_synthesis` is leftover WAITING from CA-749 park that `resumePlanApproval` never settles to DONE.

### Source Refs

- Live: `run-210188` `GET /client/workflow-runs/run-210188/agent-graph` → loop `blocked` `blockReason=escalate` `gateReason` starts `Validation failed after the maximum number of retries` / `TestDivide` panic. Parent status `waiting_question`.
- `plan_approval_park.go` `parkPlanForApproval` stamps `plan_synthesis` WAITING; `resumePlanApproval` approve path advances freeze and **never** `setFlowStepStatus(..., DONE)`.
- `tui/app/step_runtime.go` `activeStepName`: first `RUNNING` or `WAITING_USER_APPROVAL` in YAML order.
- `tui/app/step_runtime.go` Retry copy `" - run again with old scope"` unless missing-CA (CA-740).

## 1. Issue Summary

Two independent defects stacked on one live run after CA-758 passed:

1. **Stale hub WAITING.** Approve+freeze leaves `plan_synthesis` in `WAITING_USER_APPROVAL`. A later validate escalate stamps `validate` WAITING too. TUI `Now:` / footer pick the earlier hub.
2. **Dead Retry.** Validate-exhausted escalate Retry re-runs old scope. Spec conflict (`Divide` panic vs frozen `TestDivide`) cannot go green. Same card repeats. Operator reads this as an infinite gate loop.

## 2. Parent Links

- impacted coding plan: Task-325 / CP-58 dual-loop harness
- impacted tech design: flow step timeline + blocked-bar chips
- impacted system spec: none (delta on existing park/escalate UX)

## 3. Environment and Reproduction

- environment: TUI Task Harness, OpenCode Go, `D:/working/gate-sandbox`, runner `127.0.0.1:4317`
- reproduction steps:
  1. `/flow task` prompt `Change Divide(a,b) in calc.go to panic on zero divisor`
  2. Round 1 plan fail → writer rewrite → `plan_approval` → **Approve** → freeze DONE
  3. Implement; answer TestDivide question with keep-panic / compatibility
  4. Validate hits `TestDivide` panic → escalate card
  5. Click **Retry** twice or more
- frequency: 100% whenever a churned plan is approved then validate exhausts on a frozen-test conflict

## 4. Expected vs Actual

- expected:
  - After Approve+freeze: `plan_synthesis` DONE (not WAITING). `Now:` tracks the live code step (implement/validate).
  - Validate-exhausted escalate: Retry still exists for flakes; default is Revise; copy does not claim old scope will make a spec conflict pass. Freeze stays `[v]`. No `plan_approval` re-park (CA-758).
- actual:
  - `plan_synthesis` stays WAITING; `Now: plan_synthesis` while escalate is on validate
  - Retry reprints the same escalate card
  - CA-758 still true (no `Plan revised` park)

## 5. Impact

- users affected: anyone walking task-harness / bug-plan-harness past a churned plan into a red validate
- workflows affected: Task Harness code phase after plan_approval Approve
- severity: medium — flow is not hung (Stop/Revise work); chrome + Retry copy make it look hung

## 6. Root Cause

- hypothesis: leftover WAITING + first-match `Now:` + generic Retry copy
- confirmed cause:
  1. `parkPlanForApproval` stamps `plan_synthesis` WAITING. `resumePlanApproval` approve branch calls `advanceToNextInlineOrDelegate(..., "done")` + `resetPlanPhaseRound` and never settles the hub step. CA-758 skip-repark also does not settle it. Freeze DONE + hub WAITING coexist.
  2. `activeStepName` returns the first WAITING/RUNNING. YAML puts `plan_synthesis` before `validate`.
  3. Escalate Retry POSTs `/agent-loop/continue` with empty feedback ("old scope"). Validate-exhausted GateReason is unchanged → same park. Copy still says "run again with old scope" (correct for flakes, lying for spec conflict).
- evidence: live graph `blockReason=escalate` not `plan_approval`; freeze `[v]` in TUI; `resumePlanApproval` lines 213–221 have no `setFlowStepStatus(DONE)`.

## 7. Fix Strategy

- `F-1` (runner, agent-flow-engine): when `preflight_contract_freeze` becomes DONE, if `plan_synthesis` is WAITING, stamp it DONE. Hook at freeze-complete (covers Approve path, CA-758 skip-repark leftover, and any other freeze-done). Do **not** stamp DONE while freeze is pending. Log `plan_synthesis_settled_after_freeze`.
- `F-2` (TUI, cli-tui): `activeStepName` = last RUNNING if any, else last WAITING. Dual WAITING during a race still prefers the later (code-phase) node.
- `F-3` (TUI, cli-tui): if `blockedDecisionReason` matches validate-exhausted (`Validation failed after the maximum number of retries`), Retry description becomes a spec-conflict warning (Retry still POSTs continue). Default highlighted chip = Revise. Generic escalate and missing-CA (CA-740) copies unchanged.

## 8. Validation

- `V-1` New runner test: churned park → Approve → freeze DONE ⇒ `plan_synthesis` status DONE, freeze DONE, loop not `plan_approval`. Matrix Claude/Codex/Grok.
- `V-2` New runner test: freeze already DONE + leftover WAITING on `plan_synthesis` (CA-758 skip shape) ⇒ settle to DONE, no re-park.
- `V-3` New runner test: freeze **not** DONE ⇒ `plan_synthesis` stays WAITING (CA-749 lock).
- `V-4` New TUI test: steps `[plan_synthesis WAITING, validate WAITING]` ⇒ `Now: validate`. RUNNING beats earlier WAITING.
- `V-5` New TUI test: validate-exhausted GateReason ⇒ view has Revise highlighted / Retry copy not "old scope"; missing-CA and generic escalate fixtures still match CA-740 / existing copy. Matrix 3 providers like `run202550_*`.
- `V-6` Re-run related old patterns (untouched): `TestPlanApprovalPark_*`, `TestRun207435NoReparkAfterFreezeDone`, `TestBlockedBar_MissingCARetryCopy`, `TestSlashContinue_UnparksBlocked`. If any fail → STOP (R1).

## 9. Regression Guard

- tests: new files only (`bug362_plan_synthesis_settle_after_freeze_test.go`, `bug362_active_step_and_validate_retry_copy_test.go`)
- alerts: none
- audit checks: CA-761 written at implement
## 10. Follow-Up Document Updates

- upstream docs that must change: none at plan
- notes left unchanged on purpose: CA-758 residual (`activeHubNodeID` not retargeted); muse-spark ACP hang (BUG-361 F-2); question TTL 30m (CA-760); TUI leftover picker after `question_expired`

## Safe-fix header (plan)

```
feature_key: agent-flow-engine
prior CA: CA-749 (first park WAITING), CA-758 (no re-park after freeze), CA-740 (missing-CA Retry copy), CA-752 (empty=approve), CA-757 (verdict gate)
will not undo: first park, skip-repark, missing-CA copy, verdict order
R2: Case 1 agnostic — F-1/F-2/F-3 take no providerKey; matrix tests lock drift
R3: V-1..V-5 cover repro, leftover WAITING, freeze-pending lock, dual WAITING Now:, Retry copy vs CA-740
```

## Completion Notes (implemented 2026-09-07, CA-761)

- F-1 root cause refined during implement: approve already settles the hub via `markSourceDone` (old `TestPlanApprovalPark_ApproveAdvancesToFreeze` proves it). The deterministic re-stamp was `runValidateNode` max-retries calling the first-hub fallback — now stamps the validate node itself. Freeze-DONE settle helper covers approve path, CA-758 skip branch, and freeze-complete.
- F-2/F-3 as planned, plus `blockedCardAllowShown` extraction (triple-duplicated Allow logic, behavior-identical).
- V-1 drives approve through `resumeFlowWithFeedback` (real Continue path); calling `resumePlanApproval` directly on a blocked loop skips freeze via `loopIsAdvancing` — test-only lesson, production path unchanged.
- Files: runner `plan_approval_park.go` (+helper), `flow_validate_audit_dispatch.go` (F-1a + freeze-complete call), `interactive_service.go` (skip-branch call); tui `step_runtime.go` (F-2 + copy), `action_ring.go` (Revise default + helper).
- R1: 11 TUI spinner/YouBox failures byte-identical clean-vs-changed; `TestResumeFlowWithFeedbackAfterEscalate` fails identically at HEAD; zero old-test edits.
