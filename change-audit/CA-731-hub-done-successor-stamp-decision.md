# CA-731 — hub done-through-successor stamps decision; no BUG-226 escalate (BUG-353)

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-353
change_type: bugfix
summary: advanceHubDoneThroughEdge generic-successor branch (plan_synthesis -> freeze / audit) now stamps the one-decision guard like the hub.notify branch, so a successful submit_review_outcome(approved) no longer trips BUG-226 escalate; running loops report Status done + NextAction advancing (BUG-284 semantics) instead of looping
# --->8---

## Problem

- Live run-198468 (`/flow task-harness`, S6): `plan_synthesis` hub called
  `submit_review_outcome(approved)` successfully (MCP result
  `{"status":"running","nextAction":"looping"}`), but the turn then escalated
  `completed without submit_review_outcome` to WAITING_USER_APPROVAL —
  reproduced 3x including a Retry (escalate loop).
- Root cause: `advanceHubDoneThroughEdge`'s generic-successor branch (done
  edge to a real node like `preflight_contract_freeze`/`audit`, not the
  terminal) never stamped `lastFlowControlTurnID`, unlike the `hub.notify`
  branch (BUG-289). `flowControlSubmittedForTurn` stayed false, so the
  BUG-226 prose-escalate fallback fired on an already-decided turn.

## Changes

- `apps/local-runner/internal/runner/interactive_service.go`
  (`advanceHubDoneThroughEdge`): stamp `lastFlowControlTurnID =
  currentTurnID` on the generic-successor branch (mirrors the hub.notify
  branch); when the loop keeps running after the dispatch, report
  `Status:"done"` + `NextAction:"advancing"` instead of
  `Status:"running"`/`NextAction:"looping"` (BUG-284 semantics extended —
  "looping" reads to the model as a rejected decision).
- `apps/local-runner/internal/runner/bug353_hub_done_successor_stamp_test.go`
  (new): freeze-successor shape (reported repro) + audit-successor shape
  (near-miss), each asserting the stamp, duplicate-decision rejection, and
  non-"looping" result. Provider matrix over Claude/Codex/Grok via
  `registerKeyedCapture`.

## R1 (old suite untouched)

- Related pre-existing tests all green, unedited:
  `rag_harness_synthesis_edges_test`, `flow_hub_notify_test`,
  `flow_telegram_notify_test`, `task_harness_dual_loop_test`,
  `cp53_review_done_verdict_test` (10 tests incl. provider matrix),
  `bug289/bug302/bug306/bug308`, `run45103_cap_park_self_cancel_test`,
  `TestApplyFlowControlContinueHubRouting`,
  `TestCoderCompletionAutoSpawnedReviewerPromptDoesNotInstructFlowControlCall`.
- `TestSpawnChildEmitsGraphAndBusEvents` fails identically on the baseline
  (pre-existing environmental: graph snapshot present, bus event nil) —
  verified against the stashed baseline earlier today; zero new regressions.

## R2 (provider parity)

- Provider-agnostic: `advanceHubDoneThroughEdge`/`SubmitFlowControl` take no
  providerKey and never branch on one (grep evidence); every adapter's
  `submit_review_outcome` handler (Claude/Codex/Grok/Opencode) routes through
  `bridge.SubmitFlowControl` into this same path. The new matrix test locks a
  future provider-specific drift.

## Prior CA not undone

- CA-712 dual-loop routing, BUG-289 hub.notify stamp, BUG-284 "done/advancing"
  semantics, CA-616 planner model guard, CA-728/CA-730 Task-320/BUG-352 model
  carry — all intact; the change only extends the existing stamp+semantics
  contract to the sibling branch.

## Verification

- `go test ./internal/runner/ -run 'TestBug353' -count=1`: 2 new tests PASS
  (6 provider subtests).
- Related old clusters green (above). Live-verify pending: rebuild runner,
  `/flow task-harness` + GCD prompt → `plan_synthesis` approve →
  `preflight_contract_freeze` advances (S6 PASS).