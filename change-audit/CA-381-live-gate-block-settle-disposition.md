# CA-381 — Live gate block/reprompt settle disposition fix

## Summary

Fixed BUG-301: a live turn whose post-turn gate blocked/reprompted (Tier-1
doc-scope violation, e.g. missing bugfix doc / missing change-audit note) left
its dispatch record's `SettlePhase` stuck at `settle_pending` forever — the
"Dispatch attention" card never cleared, even after the overall run reached
"Completed". Only a full server restart's boot recovery sweep
(`drivePendingSettlesOnBoot` → `resumePendingFlowGate`) would eventually
resolve it.

`resumePendingFlowGate`'s own block branch (the boot/resume path) already
called `scheduleSettleAfterGateBlock` after persisting the reprompt
checkpoint. The live post-turn-gate block branch in `runTurn`'s tail — reached
the very first time a turn's gate resolves, no restart needed — never had the
matching call, unlike its sibling live gate-**pass** branch, which already
called `scheduleSettleAfterGatePass`. Added the missing call at the same
point (immediately after the reprompt/block checkpoint persists), mirroring
the existing boot/resume-path behavior exactly.

## Cross-provider parity

Classification: dispatch/settle plumbing, provider-agnostic. The fixed branch
does not read or branch on `ProviderKey`; it fires for any flow-engine-driven
root or child turn whose post-turn gate blocks, regardless of which provider
ran the turn.

## Verification

- New regression test `TestLiveGateBlockSchedulesSettleDisposition` (real temp
  git repo, genuine r-ca Tier-1 violation, driven through the real `runTurn`
  path) — confirmed to fail (stuck at `settle_pending`, reproducing
  run-18997/turn-19161) without the fix and pass with it.
- Full gate/dispatch/settle regression battery (227 tests across
  `TestDispatch*`, `TestGateSettle*`, `TestChildGate*`, `TestRunTurnGate*`,
  `TestFlowGate*`, `TestBug288*`/`TestBug298*`, CP-51 Phase-A bundle, etc.)
  unchanged, all passing.
- `go build ./...` passes.

## Root cause vs symptom

The symptom (a lingering, harmless-looking "Dispatch attention" card) masked
a real asymmetry: gate-pass and gate-block are structurally identical outcomes
for settle purposes (both need a durable final disposition once their
checkpoint persists), but only one of the two live branches — and only one of
the two resume-path branches — had ever been wired to the settle scheduler.
This closes that gap for the live path, matching the resume path's own
already-correct behavior.

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: BUG-301
change_type: bugfix
summary: Call scheduleSettleAfterGateBlock from the live post-turn-gate blocked/reprompt branch, matching the already-correct boot/resume path and the sibling live gate-pass branch, so a live gate block/reprompt no longer leaves its dispatch record stuck at settle_pending until a server restart.
# --->8---
