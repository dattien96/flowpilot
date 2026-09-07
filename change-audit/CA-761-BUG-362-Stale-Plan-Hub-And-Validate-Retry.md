# CA-761 — BUG-362: stale plan hub WAITING + validate-exhausted Retry loop

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-362
change_type: bugfix
summary: Validate-escalate stamps its own node instead of the first hub, freeze DONE settles stale plan_synthesis WAITING, Now: prefers the live step, validate-exhausted Retry warns and defaults to Revise
# --->8---

## Safe-fix header

- Prior CA read: CA-749 (first churned park WAITING), CA-758 (no re-park after freeze DONE), CA-740 (missing-CA Retry copy), CA-752 (empty=approve), CA-757 (verdict gate first), CA-741 (park-cancel), CA-353/289 (one-decision guard).
- Will-not-undo: first park before freeze; skip-repark after freeze DONE; missing-CA copy; empty=approve; verdict gate order; one-decision stamp.
- R2: Case 1 agnostic — F-1/F-2/F-3 take no `providerKey` and never branch on one; new tests matrix Claude/Codex/Grok anyway to lock drift.

## Live repro (run-210188, TUI Task Harness, gate-sandbox)

- Churned plan approved → freeze `[v]` → implement → TestDivide question answered keep-panic → validate hit `TestDivide` panic → `blocked/escalate` "Validation failed after the maximum number of retries".
- CA-758 held (no `Plan revised` re-park). Operator clicked Retry repeatedly → same card (old scope cannot fix a spec conflict).
- Sidebar showed `plan_synthesis WAITING_USER_APPROVAL` + `validate WAITING_USER_APPROVAL`, `Now: plan_synthesis`, footer `step: plan_synthesis` while the live gate was validate.

## Root cause (confirmed in code, not just the plan's hypothesis)

- The plan's F-1 (approve never settles the park stamp) was only half: `markSourceDone` already settles plan_synthesis DONE on the approve dispatch (old test `TestPlanApprovalPark_ApproveAdvancesToFreeze` asserts it).
- The deterministic re-stamp: `runValidateNode` max-retries called `setFlowStepAwaitingUser` BEFORE `applyFlowControl`, and that helper stamps the FIRST `hub.inline` — `plan_synthesis` on dual-hub flows — resurrecting the dead hub beside the correctly-stamped validate. `activeStepName` then picks the first WAITING.

## Changes

- `flow_validate_audit_dispatch.go` `runValidateNode` max-retries: stamp the validate node itself (`setFlowStepStatus(..., node.ID, WAITING)`); `applyFlowControl` re-stamps the same node via `lastEscalated` (idempotent). The first-hub fallback call is gone on this branch.
- `plan_approval_park.go`: new `settlePlanSynthesisAfterFreezeDone` — WAITING→DONE only when freeze reads DONE; RUNNING/PENDING/DONE untouched; pending freeze is a no-op (CA-749 lock).
- Call sites: `runContractFreezeNode` success (every freeze completion) + CA-758 skip branch in `advanceHubDoneThroughEdge` (stale done after freeze).
- `tui/app/step_runtime.go`: `activeStepName` = last RUNNING, else last WAITING. Blocked bar: validate-exhausted gate (`maximum number of retries`) keeps `[Retry]` for flakes but copy becomes `re-run unchanged (spec/test conflict fails again)`; plan_approval and missing-CA copies unchanged.
- `tui/app/action_ring.go`: `syncActionRingCard` defaults Enter to Revise on a fresh validate-exhausted card (arrows still move; reset only on card change). Extracted `blockedCardAllowShown` shared by bar, ring items, and `blockedReviseRingIndex` (was triple-duplicated, behavior-identical).

## Deliberately not changed

- No `activeHubNodeID` retarget (CA-758 residual stands).
- No turn-level timeout for muse-spark ACP hang (BUG-361 F-2).
- TUI leftover picker after `question_expired` (separate UX gap).
- Retry still POSTs continue with old scope — flakes still recover that way.

## Tests (new files only)

- `bug362_plan_synthesis_settle_after_freeze_test.go` (matrix ×3): V-1 park→Continue-approve⇒freeze DONE + plan_synthesis DONE + unblocked; V-2 CA-758 skip with stale WAITING⇒advancing + DONE; V-3 freeze pending⇒helper no-op (still WAITING); F-1a validate-exhaust on dual-hub post-approve shape⇒validate WAITING, plan_synthesis/freeze stay DONE, loop escalate with exhausted copy.
- `bug362_active_step_and_validate_retry_copy_test.go` (matrix ×3): V-4 `activeStepName` table (dual WAITING→later, RUNNING beats WAITING either order, singletons/empty unchanged); V-5 exhausted copy + no old-scope claim + ring has Retry/Stop/Revise with Enter on Revise (idx 2); generic escalate copy + Retry default unchanged.
- V-6 old suites green untouched: `TestPlanApprovalPark_*`, `TestRun207435*`, `TestCP61HubDone*`, `TestCP53ReviewDoneVerdict*`, `TestFlowRequiresSynthesisMachineVerdict`, `TestPlanPhaseRoundReset*`, validate/freeze/escalate/drift neighbors, full `tui/app` step/ring/blocked patterns.

## R1 pre-existing failures (stash-proven, untouched)

- `tui/app` full suite: 11 failures (spinner/YouBox rendering) byte-identical on clean tree vs this tree (`diff` clean).
- `TestResumeFlowWithFeedbackAfterEscalate` (`phase_a_dod_test.go:200`, review-loop resume hub RUNNING) fails identically at HEAD — predates this session, different path (no freeze/validate/TUI involvement).
- `TestOpencodeProcessEnvIsolatesWindows` (Windows env path, proven in BUG-361 session).

## Prior CA intact

- CA-749 first park; CA-758 skip-repark (V-2 builds on it); CA-740 missing-CA copy (V-5 pins it); CA-752 empty=approve (V-1 drives it); CA-757 verdict order; CA-353 guard stamp.
