# CA-1078: working-mode gate resolves catalog UUID → pack flow identity

Date: 2026-10-01
Refs: CP-90 (vibe-tasks entry), Task-326 T-1/T-3 (working-mode gates),
BUG-174 (resolveWorkflowFlowRef bridges workflowID → canonical flowRef),
BUG-270 (resolver validation error channel), live report:
`working_mode_flow_forbidden` (HTTP 400) on Vibe + Flow-mode "Vibe Tasks".

## Problem

`enforceWorkingModeStart` gates the effective start ref verbatim:

```go
flowID := in.FlowRef; if flowID == "" { flowID = in.WorkflowID }
FlowAllowedForWorkingMode(mode, flowID, "user")
```

A Flow-mode picker launch sends `workflowId` = the `workflows` row's
catalog **UUID** (e.g. `916056ab-…` for the builtin `vibe-tasks` mirror),
not the pack identity. `BareFlowID` cannot map a UUID → in Vibe the
user-start gate sees an unknown id → **400 working_mode_flow_forbidden**.
The chat-armed entry (bare `vibe-tasks` flowRef) never hit this, which is
why the picker path was the only broken door. Dev mode passed the same
UUID by design (catalog UUIDs are untracked ids), so the bug was
vibe-only at create — but in dev the canonical ref only got gated one
step later at turn admission (`handleStartTurn` resolves the UUID via
`resolveWorkflowFlowRef` first, then gates the resolved ref).

The forward path had the same hole with a second consequence:
`resolvePinnedFlowRefForForward` returns `rs.workflowID` (the UUID)
verbatim → `runFirstTurnFences` gated the UUID (400 in vibe) and, had it
passed, `commitPendingFlowStartLocked` would stamp the UUID into
`chatFlowRef`, silently missing every downstream `BareFlowID` consumer
(`vibeAwaitingLock`, `isVibeCpSourcedFlowID`, checkpoint/drift hooks).

## Fix

Two sites, one shared semantics — resolve the catalog identity through
the existing `FlowDefinitionResolver` seam before gating/committing:

1. `enforceWorkingModeStart` (`working_mode_http.go`): when the start ref
   resolves to a flow record (UUID → mirror row, bare/pack ref →
   canonical), gate `record.FlowRef`. On any resolution failure the raw
   value is gated — unchanged behavior: dev still admits arbitrary
   catalog workflows, vibe still fails closed on unknown ids.
2. `forwardPinnedFlow` (`interactive_service.go`): normalize the pinned
   ref (explicit turn `flowRef` or `rs.workflowID` fallback) to the
   canonical pack ref before the latch checks, so `runFirstTurnFences`
   gates the real identity and the committed `chatFlowRef` is canonical
   — restoring the `vibeAwaitingLock`/CP-source markers for vibe pins.

Fail-closed preserved: resolving a `vibe-sprint`/`vibe-owner-debate`
mirror UUID now rejects at **create** (system flow as user start) instead
of later at turn admission; unresolvable UUIDs still 400 in vibe.

## Files

- `internal/runner/working_mode_http.go` — resolve-before-gate.
- `internal/runner/interactive_service.go` — pin normalization in
  `forwardPinnedFlow`.
- `internal/runner/ca1078_workflow_uuid_mode_gate_test.go` — 6 cases:
  vibe+UUID→vibe-tasks admitted (was 400), vibe+UUID→vibe-sprint
  forbidden, vibe+unknown-UUID forbidden, dev+UUID→vibe-tasks forbidden,
  forward pin returns canonical ref (was 400), forward pin on
  vibe-sprint UUID forbidden.

## Verification

- Red first: `…ResolvesToVibeTasks` and `…ResolvesCanonical` failed with
  the live `400 working_mode_flow_forbidden`; `…Dev…Forbidden` was nil
  (documented the latent dev create-gate gap).
- Green after; gate/regression families
  (`WorkingMode|FlowAllowed|FlowRef|Forward|Vibe|Mirror`) pass;
  `go vet` clean.
