# CA-647 — Requirement-Class Gate Park Made Visible and Hub-Targeted (BUG-1196)

**Date**: 2026-10-05
**Author**: Devin
**Ticket**: BUG-1196

---

## Problem

Live `run-225691` (PrivateVault Task-0310 sprint), twice in one afternoon:

- `turn-254226` @12:51:57 (+07): hub reinvoke gate-blocked on
  `r-newtest + r-additive-tests + r-requirement`. The requirement-class
  violation routed `vibeGateRequirement` — the SS-18 BR-4 user-only
  block — which set `loop.Status=blocked` + `BlockReason=requirement` in
  memory and emitted one fire-and-forget `flow_gate_violation` event.
  No step stamp, no `agent_graph_updated`, no `parkFlowForAwaitingUser`
  freeze, and no durable decision (`pendingGateBlock` only arms on r-reg
  options). The flow sat invisibly frozen ~20 minutes until the hub-stall
  watchdog surfaced a generic card.
- `turn-260646` @14:14:40 (+07): identical rules, identical dead end —
  required a manual resume again.

Two defects in `applyVibeGateResolver`'s requirement route:

1. **Invisible park**: the blocked loop existed only in memory — no
   step status (`hub.inline` stayed PENDING/RUNNING), no graph emit, no
   `flow_parked_awaiting_user` diag, no persisted park. A client that
   missed the single violation event observed a silent freeze.
2. **Wrong loop target for gated children**: `mutateLoop(runID)` ran on
   the gated child run id — children own no loop state — leaving the
   parent hub `running` while the member silently waited: the same
   invisible freeze one level down, plus a guaranteed later
   `member_stalled` misfire.

## Fix

`vibeGateRequirement` now applies the established escalate park contract
(Task-240 I-1/I-2 ordering) aimed at the **hub** (`parentID` when the
gated run is a child):

- gated run → `waiting_user_approval` + violation event (unchanged
  surfaces), plus `stampEscalatedChildNodeLocked` for children so
  Continue retries the gated node instead of a generic hub reinvoke
  (mirrors the escalate path).
- `setFlowStepStatus(escalatedNode, WAITING)` for a gated child, else
  `setFlowStepAwaitingUser(hub)` — the step runtime now shows the park.
- `mutateLoop(hub → blocked/requirement/gateReason)` — the hub's loop,
  never a child's.
- `parkFlowForAwaitingUser(hub)` — real freeze: drops armed hub intents,
  cancels in-flight sibling work (a requirement conflict means the spec
  is broken; coding behind the card is waste), and preserves the gated
  child's intents because it is already `waiting_user_approval`.
- `emitAgentGraph` + `persistParentSession` — the park is pushed and
  durable, so reconnect/poll surfaces it without waiting on the stall
  watchdog. `flow_parked_awaiting_user` diag for parity.

## Files

- `internal/runner/vibe_gate.go` — `vibeGateRequirement` rewritten onto
  the hub-targeted visible-park contract.
- `internal/runner/bug1196_requirement_park_surface_test.go` —
  red→green tests.

## Test evidence

- `TestBug1196_RequirementParkStampsAndFreezes` — red before the fix (hub
  step stayed PENDING while the loop blocked); green after: loop
  blocked/requirement, run waiting, hub step WAITING_USER_APPROVAL.
- `TestBug1196_ChildRequirementParkTargetsParentLoop` — red before
  (parent loop stayed `running`: the park mutated the child's loop);
  green after: parent loop blocked/requirement, child's node stamped
  WAITING, `lastEscalatedInlineNodeID=implement` recorded for Continue.

Provider parity: provider-agnostic — gate-resolve bookkeeping only.

Regression: `go test -run 'Bug1196|Bug1194|Bug1195|Vibe|Gate|Escalat|
Requirement|Stall|Cohort'` → ok (56.8s).
