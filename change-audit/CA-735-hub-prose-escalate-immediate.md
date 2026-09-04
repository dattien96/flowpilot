# CA-735 — hub prose-synthesis escalates immediately, no 2m hub_stalled (run-199617)

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-58
change_type: bugfix
summary: BUG-226's prose-escalate fallback now treats a flow-engine hub's terminal provider event (EventTurnCompleted) as "turn finished", so a hub synthesis turn that completes without submit_review_outcome escalates immediately to an actionable card instead of idling 2 minutes into hub_stalled (run-199617 S6)
# --->8---

## Problem

- Live run-199617 (`/flow task-harness`, S6, target `D:\working\gate-sandbox`): `plan_reviewer` returned APPROVED, cohort joined, `plan_synthesis` hub was reinvoked (turn-199952). The TUI showed a `submit_review_outcome` tool call, but no `flow_control_received` ever reached the engine; the hub finished the turn in prose ("Consolidated plan review: **APPROVED** … Advancing.").
- The step sat RUNNING until the 2m hub watchdog parked `blocked/hub_stalled` (`hub has made no progress for 2m0s (no turn, gate, or reinvoke in flight)`). Retry re-runs the same model, so the loop repeats instead of surfacing an actionable card.
- Root cause: BUG-226's gate used `completed` from `finishTurn`, defined as `rs.status == RunStatusCompleted || rs.pendingFlowGateSettle`. A flow-engine hub stays `status=running` while the loop is live, and `pendingFlowGateSettle` is only armed for children (`parentRunID != ""`), so `completed` was false even though the provider turn itself finished via `EventTurnCompleted`. BUG-226 never fired → silent RUNNING → 2m watchdog.
- CA-731/BUG-353 (the *other* direction: tool WAS called but not stamped) already fixed; this is the *no tool call* class, where BUG-226's escalate is the intended behavior — it just never ran on a live hub.

## Changes

- `apps/local-runner/internal/runner/interactive_service.go` (BUG-226 gate): `hubTurnFinished := completed || rs.lastEventType == EventTurnCompleted` — a terminal provider event on the hub counts as "the turn finished" for the prose-escalate fallback. All existing guards still apply unchanged:
  - tool submitted for this turn (`flowControlSubmittedForTurn`) → no escalate;
  - open cohort / live child (`hubShouldSkipProseEscalate`, run-5296/CA-360) → no escalate;
  - loop already sealed (`loopAlreadySealedAtTurnStart`, BUG-302/308) → no tool offered → no escalate;
  - hub's first pre-cohort turn (`turnCount == 1`) → tool not offered → no premature escalate.

## Tests added (new file only)

- `run199617_hub_prose_escalate_immediate_test.go`:
  - `TestRun199617HubProseWithoutToolEscalatesImmediately` (provider matrix Claude/Codex/Grok): drives the hub through two real turns — turn 1 (pre-cohort, prose) must NOT escalate; turn 2 (synthesis, prose without the tool) must BUG-226 escalate immediately: `plan_synthesis` → WAITING_USER_APPROVAL, loop blocked with "completed without calling submit_review_outcome", and NOT `hub_stalled`.
  - `TestRun199617OpenCohortStillSkipsProseEscalate` (near-miss, CA-360 guard): with an open reviewer cohort, the hub's prose turn still skips escalate — the widened "turn finished" signal must not bypass the open-cohort skip.

## Verification

- `go test ./internal/runner/ -run 'TestRun199617|TestBug353|TestHubShouldSkip|TestRun5296|TestRun45103|TestFlowSettle|TestRun198699|TestTaskHarness|TestApplyFlowControl|TestResolveContinueBackEdge|TestBug327|TestCoderCompletionAutoSpawned|TestRagHarness|TestRun2047|TestV9|TestRun135037|TestBug302|TestBug308|TestRun1618' -count=1` PASS. Old tests untouched (R1).
- Provider-agnostic: the gate does not branch on provider; matrix covers all three (R2).
- `go vet ./internal/runner/` clean; runner binary builds.