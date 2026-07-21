# CA-392: Flow/Workflow YOLO force on create, rehydrate, and follow-up

## Summary

Closed the residual BUG-299 gap surfaced on live `run-35329` (Grok ACP approval card after a Flow follow-up): after a Flow reached done/cancel, a follow-up prompt (and any runner restart/rehydrate) could run with `rs.yolo=false` even though product policy requires YOLO always on for Flow/Workflow. Chat mode continues to honor the UI toggle.

Root causes (shared across Claude/Codex/Grok — not Grok-only):

1. Desktop workflow launches omit `yoloMode` on every turn (`store.ts`: only `normal_chat` sends the toggle).
2. `sessionStateOf` never persisted `yolo`, so restart reconstructed `rs.yolo=false`.
3. Catalog/DB rows could still be `yolo_mode=false` (pre-CA-378 default); createRun only OR'd the catalog value.

## What Changed

- `yolo_resolver.go`: `shouldForceFlowYolo` / `resolveEffectiveYolo` — force true for `flowEngineDriven`, non-empty `workflowID`, or `runKind` workflow/empty; pure chat keeps base.
- `createRun`: apply force after catalog resolution; initial session persist writes `Yolo`.
- `runTurn`: apply force so follow-ups with omitted `yoloMode` still send `TurnRequest.YoloMode=true`; stick `rs.yolo`.
- First-turn `flowRef` path: set `rs.yolo=true` when marking `flowEngineDriven` so async child spawn inherits the lock.
- `reconstructRunInternal`: restore `st.Yolo` then re-apply force (legacy missing field → force true for flow).
- Durable round-trip: `ProviderSessionState.Yolo`, `sessionStateOf`, local NDJSON + Supabase `sessionRuntimeBlob`.

ForceShellBridge + commit denylist unchanged (`resolveYoloPostureForTurn` still forces bridge modes under YOLO for coding children).

## Tests

- New additive file `bug299_flow_yolo_force_rehydrate_test.go`: force matrix, createRun catalog-false, chat toggle, post-done follow-up, reconstruct paths.
- New additive file `bug299_flow_yolo_coverage_matrix_test.go` (investigate checklist):
  - Claude + Codex + Grok follow-up after done → `TurnRequest.YoloMode=true`
  - Follow-up after **stopped** loop still forces yolo
  - Chat YOLO off → still emits `permission_required` card
  - Chat YOLO off + policy denylist still denies without card
  - Coding commit still **deny** under forced YOLO; ordinary shell still auto-approve
  - Blocked coding child still denies commit (no wrong auto-approve)
  - Local NDJSON session `Yolo` round-trip + reconstruct after disk reload
  - `sessionStateOf` includes Yolo; createRun workflow persists Yolo=true
  - `resolveEffectiveYolo` chat toggle unit table
- **Legacy tests updated** (product supersession, not silent weaken):
  - `TestCreateRunDoesNotApplyEntryStepYoloToWorkflowLaunch` → expect force-true under BUG-299 residual.
  - `TestPolicyAutoDenyNoCard` / `TestPolicyAutoApproveNoCard` → use `normal_chat` YOLO=off (policy allow/deny is unreachable under forced workflow YOLO; was flaking to `yolo_gating_disabled`).

## Verification

```text
go test ./internal/runner -run 'TestShouldForceFlowYolo|TestCreateRunForcesYolo|TestCreateRunChatStillRespects|TestFlowFollowUpAfterDone|TestFlowFollowUpAfterStopped|TestChatYoloOff|TestFlowCodingCommitStillDenied|TestBlockedCodingChild|TestLocalFileSessionStoreYolo|TestReconstructAfterDisk|TestSessionStateOfIncludesYolo|TestCreateRunWorkflowPersistsYolo|TestResolveEffectiveYolo|TestReconstructRunForcesYolo|TestCreateRunDoesNotApplyEntryStepYolo|TestCreateRunResolvesYolo|TestRequestApprovalDeniesFlowCodingCommit'
```

All pass.

## cross-provider-parity

Case 1 provider-agnostic: force path has no `providerKey` branch; shared `TurnRequest.YoloMode` → adapters. Grok was the live symptom (request_permission + compound shell) but Claude/Codex share the same residual.

## durable-replay-contracts

Session YOLO field is additive; force covers missing legacy rows for flow. Chat restores durable true/false when present.

# ---8<--- flowpilot:change-ledger
feature_key: yolo-policy
source_doc_id: BUG-299
change_type: bugfix
summary: Force YOLO=true for Flow/Workflow on create, rehydrate, and follow-up; persist session yolo for chat stickiness without reopening flow YOLO-off
# --->8---
