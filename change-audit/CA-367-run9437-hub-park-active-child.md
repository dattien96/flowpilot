# CA-367 - run-9437 hub park while flow children active

## Summary

Implements CP-51 §3.8 A1 residual DoD: the flow hub is parked for write/tool/gate-reprompt turns while any flow child is still `RUNNING` or waiting, and a same-turn hub gate reprompt after `flow_control continue` that already delegated a writer is suppressed.

- `shouldParkHubWriteTurn` / `scheduleRootGateRepromptOrPark` (shared path; no provider branch).
- `startTurn` rejects hub turns with `hub_parked` when children are active.
- `applyFlowControl(continue, looping)` calls `stampHubContinueDelegatedDurable`: sets `HubContinueDelegatedTurnID`, clears `PendingGateReprompt*`, and persists both atomically.
- `hub_parked` (and `flow_awaiting_user`) are transient for durable-intent delivery.
- Child idle path flushes parent durable intents once no active children remain.
- Session field `HubContinueDelegatedTurnID` round-trips via local NDJSON + Supabase runtime blob; reconstruct restores it; idle flush durable-clears any revived reprompt under the marker.

Does not serialize graph delegate fan-out or change hub.inline `flow_control` waits.

## Codex review pass 1 fix

Same-turn continue suppression was RAM-only. Root gate paths could leave `PendingGateReprompt*` on disk while the marker was not session-backed, so restart revived the forbidden hub reprompt. Fixed by durable marker + atomic clear+persist of `PendingGateReprompt*` on stamp and suppress paths, with restart/idle-flush coverage.

## Codex review pass 2 fix

Marker was never consumed, so idle flush suppressed later hub gate reprompts (N+1). Suppress is now strictly turn-scoped (`turnID == hubContinueDelegatedTurnID`), consumes the marker on same-turn suppress, and clears a stale marker when a new hub turn starts. Flush uses `pendingFlowGateTurnID`/`lastTurnID` as the gate identity (not the marker itself).

## Follow-up fix: stranded hub.notify reinvoke (runTurn postTurnGateCancel)

`TestHubNotifyDeferredReinvokeRetriesWithStashedPromptNotGenericOne` (pre-existing, unchanged) was failing: the deferred hub.notify reinvoke (armed via `maybeAutoReinvokeHubWithPrompt` while `turnInFlight` was still true) never got its second hub turn scheduled.

Root cause in `runTurn` (`interactive_service.go`): on the two "gate passed" success branches for a run with `parentRunID == ""` (root hub) — the child-settle branch and the no-deferred-settle branch — `rs.turnInFlight` was cleared but `rs.postTurnGateCancel` was left set; it is only nilled by a `defer` that runs at function return. `runTurn`'s tail unconditionally calls `notifyTurnIdle(rs.id)` *before* that defer fires, and `notifyTurnIdle`'s busy check (`rs.turnInFlight || rs.pendingFlowGateSettle || rs.postTurnGateCancel != nil`) saw `postTurnGateCancel != nil` and treated the run as still busy, skipping the `pendingHubReinvoke`/`pendingHubReinvokePrompt` drain. No later event re-triggers `notifyTurnIdle`, so the stashed hub.notify prompt was stranded forever. The sibling branches (gate-blocked / epoch-mismatch / cohort-skip) already cleared `postTurnGateCancel` explicitly and were unaffected.

Fix: explicitly set `rs.postTurnGateCancel = nil` in both success branches (interactive_service.go, `runTurn`) before unlocking, matching the sibling branches. No test changes — the pre-existing regression test now passes.

## Verification

- New files only: `run9437_hub_park_active_child_test.go`, `run9437_hub_park_restart_suppress_test.go` (additive-tests-only).
- Restart coverage uses session-store reload + `reconstructRun` (not `resumeRun`, which requires provider-home transcripts).
- Focused `go test` for the new patterns (see agent report).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: bugfix
summary: Durable hub park + continue-delegate suppress of root gate reprompt across restart (run-9437 A1 residual).
# --->8---

