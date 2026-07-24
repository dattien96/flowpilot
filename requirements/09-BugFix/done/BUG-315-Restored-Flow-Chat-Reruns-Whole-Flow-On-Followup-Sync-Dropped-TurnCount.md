# BUG-315: Restored Flow Chat Re-Runs Its Whole Flow On The Next Follow-Up — Sync Dropped TurnCount

## Metadata

- Document ID: `BUG-315`
- Title: `A Drive-restored flow-hub chat re-spawns its entire flow (coder + reviewers + synthesis) on the next plain follow-up turn`
- Phase: `bugfix`
- Status: `done`
- Owner: `agent-flow-engine`
- Reviewers: `TBD`
- Created: `2026-07-23`
- Last Updated: `2026-07-23`
- Parent Documents: [CP-51: Durable Turn Dispatch State Machine And Recovery Reconciliation](../../07-Coding-Plan/done/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [CP-51 Phase A/B Timeline And Verification Log](../../07-Coding-Plan/inprogress/CP-51-PhaseAB-Timeline-And-Verification-Log.md)
- Child Documents: `none`
- Related Documents: [BUG-313: Restored Chat Timeline Broken — Sync Never Carried The Turn Log](../done/BUG-313-Restored-Chat-Timeline-Broken-Sync-Never-Carried-Turn-Log.md) (same manifest, same class: sync dropped a field the restored run needs to behave like a same-machine resume), [BUG-314: Single-Reviewer Reinvoke Round-2 Agent Cards Lost On Restart](../done/BUG-314-Single-Reviewer-Reinvoke-Round2-Agent-Cards-Lost-On-Restart.md) (found in the same restore-parity investigation), [Task-190: Chat Drive Sync](../../08-Task/done/Task-190-Cross-Machine-Chat-Session-Drive-Sync.md) (the manifest this bug extends)
- Replaces: `none`
- Tags: `agent-flow-engine, google-drive, chat-sync, restore, flow-restart, cross-provider, regression, severity-high`

## AI Quick View

### Summary

- Operator deleted 3 flow-hub chats (Codex/Claude/Grok), restored them from Drive, then sent each a plain follow-up ("hi, turn trước bạn làm gì?"). All three **re-ran their entire flow** — a fresh coder (and for the cohort flows, the reviewers + synthesis) spawned again, instead of the follow-up reaching the hub as a normal message.
- Root cause: the flow's entry nodes are started only on a run's **genuine first turn**, gated on `rs.turnCount == 0` (in `startTurn`, and again in `resolveWorkflowFlowRef`, which re-resolves the flowRef from the still-restored `workflowID`). The Drive sync manifest carried every other flow-runtime field (LoopState, ActiveFlowNodes/Edges, ChatFlowRef, ChatSubMode, TurnLog…) **but not `TurnCount`** — so a restored hub came back with `turnCount = 0`, both guards misfired, and the follow-up looked exactly like a brand-new chat's first turn.
- Restart (same machine) was never affected: `reconstructRun` reads `TurnCount` from the **local** session store, which persists and restores it correctly. Only the Drive-restore path dropped it.

### Current Ask

- Fixed. `ChatSessionSyncManifest` now carries `TurnCount`; `BuildChatSessionSyncManifest` attaches it and `restoreChatRunTreeFromDrive` writes it back onto the restored session. Plus a `restoredFrom` guard in both `startTurn` and `resolveWorkflowFlowRef` so a restored run never re-starts its flow even from a pre-fix manifest that still lacks the count.

### Key Decisions

- `V-1` **Carry TurnCount** as the primary fix (not just a guard): `turnCount` gates more than flow re-trigger — a restored run with `turnCount==0` also mis-ran `offerReviewOutcomeTool` (`rs.turnCount > 1`) and the turn mode-prefix (`prependModePrefix(…, rs.turnCount, …)`). Restoring the real count fixes all three; it is the field's correct value, not a workaround. Mirrors exactly how every other flow-runtime field already round-trips through the manifest.
- `V-2` **restoredFrom guard** in both flow-start paths (`startTurn` inline condition; `resolveWorkflowFlowRef` early bail). A Drive-restored run is by definition a continuation whose flow already ran on the source machine — it must never treat a follow-up as a first-turn flow start. This is belt-and-suspenders on top of V-1 and, crucially, heals chats **synced by a pre-fix manifest** (which still restore with `turnCount==0`). Safe: a chat that never took a turn cannot have been synced, so `restoredFrom != ""` never coincides with a legitimate first-turn flow start. `flowEngineDriven` is independently restored by `reconstructRun` (via ActiveFlowNodes), so skipping the block does not demote a restored hub to a plain chat.
- `V-3` No manifest schema-version bump: `omitempty`, absent == 0 == pre-fix behavior; old runners ignore the field.
- `V-4` Provider-agnostic by construction: no `providerKey` branch anywhere; the round-trip test runs Codex, Claude, and Grok.

### Constraints

- Backend-only; no desktop change.
- Does not change the live (non-restart) path: a genuinely new chat's first turn still starts the flow (proven by the non-restored control in the guard test).

### Open Questions

- None for the defect.

### Source Refs

- `apps/local-runner/internal/runner/chat_session_sync.go` — `ChatSessionSyncManifest.TurnCount`; `BuildChatSessionSyncManifest` (attach); `restoreChatRunTreeFromDrive` restore-session build (write back).
- `apps/local-runner/internal/runner/interactive_service.go` — `startTurn` first-turn flow-start block (`turnCount==0` + new `restoredFrom==""` guard); `TurnCount` persisted at `sessionStateOf` and restored at `reconstructRunInternal`.
- `apps/local-runner/internal/runner/flow_executor.go` — `resolveWorkflowFlowRef` (new `restoredFrom` early bail).
- `apps/local-runner/internal/runner/bug315_restore_turncount_no_reflow_test.go` — the new tests (§8).
- Live evidence: hubs `run-53157`/`run-46797`/`run-45881` restored with `turn_count` reset to 0 (came back `=1` after one follow-up) and each spawned a fresh coder child (`run-33597`/`run-33588`/`run-33606`) on that follow-up.

## 1. Issue Summary

Restoring a flow-engine (Review Loop / Bug sub-mode) chat from Drive and then sending it any plain follow-up message re-spawned the whole flow instead of continuing the conversation at the hub. The user's own prompt was treated as a fresh "start the flow" trigger.

## 2. Parent Links

- impacted coding plan: [CP-51 Phase A/B Timeline And Verification Log](../../07-Coding-Plan/inprogress/CP-51-PhaseAB-Timeline-And-Verification-Log.md) — the restore-side quality of C6 (Drive sync round-trip); the follow-up-after-restore behavior sits directly on the path C6 exercises.
- impacted tech design: `none directly` — extends Task-190's chat-sync manifest with one more parity field.
- impacted system spec: `none known`.

## 3. Environment and Reproduction

- environment: any flow-engine chat (Review Loop, Bug sub-mode, or any run with a `workflowID`/`chatFlowRef` that resolves to a flow with a spawnable entry node) restored from Drive.
- reproduction steps:
  1. Run a flow chat to completion (hub takes ≥1 turn), sync it to Drive.
  2. Delete the local chat, restore it from REMOTE CHATS.
  3. Send any plain follow-up message.
  4. expected: the follow-up reaches the hub and gets a direct answer (a normal continuation turn).
  5. actual: the entire flow re-spawns — a new coder (and for cohort flows, reviewers + synthesis) runs again from scratch.
- frequency: 100% on the first follow-up after a Drive restore of a flow chat.

## 4. Expected vs Actual

- expected: "restore từ Drive rồi chat tiếp" behaves like "restart server rồi chat tiếp" — the flow does not auto-re-run unless explicitly requested.
- actual: parity held for the restart path (local `TurnCount` preserved) but not the restore path (manifest dropped `TurnCount` → `turnCount=0` → first-turn flow start re-fires).

## 5. Impact

- users affected: anyone continuing a restored flow chat, or continuing a flow chat on a second machine.
- workflows affected: REMOTE CHATS restore of any Review Loop / Bug-mode chat; CP-51 C6 restore-side.
- severity: high — a plain "hi" silently burned a full multi-agent flow run (provider quota + wall-clock) and buried the user's actual question under a re-run they never asked for.

## 6. Root Cause

- confirmed cause: `startTurn` starts the flow only when `rs.turnCount == 0 && in.FlowRef != ""` ([interactive_service.go](../../../apps/local-runner/internal/runner/interactive_service.go)); `in.FlowRef` is re-populated on a follow-up by `resolveWorkflowFlowRef`, which itself only resolves when `rs.turnCount == 0`. Both guards key on `turnCount`. `reconstructRun` restores `turnCount` from the persisted session state (`st.TurnCount`), and the local store persists it — so a same-machine restart is correct. But `ChatSessionSyncManifest` never carried `TurnCount`: `BuildChatSessionSyncManifest` didn't set it and the restore-side session build didn't read it (the whole file had zero `TurnCount` references). A Drive-restored hub therefore came back with `turnCount = 0`, both guards fired, and the follow-up ran as a first turn → `startResolvedFlow` re-spawned the entry nodes.
- evidence: the 3 restored hubs (`run-53157`/`run-46797`/`run-45881`) all showed `restored_from=google_drive`, `turn_count=1` (0 restored + 1 from the follow-up) despite having taken 5/4/2 hub turns originally; each follow-up spawned a fresh coder child (`run-33597`/`run-33588`/`run-33606`). All new tests fail on pre-fix HEAD with the exact defect (`manifest turnCount = <nil>`; restored run resolves the flowRef).

## 7. Fix Strategy

- `F-1` `ChatSessionSyncManifest` gains `TurnCount int` (`json:"turnCount,omitempty"`).
- `F-2` `BuildChatSessionSyncManifest` sets `TurnCount: session.TurnCount`; restore-side session build sets `TurnCount: manifest.TurnCount`.
- `F-3` `startTurn`'s first-turn flow-start condition gains `&& strings.TrimSpace(rs.restoredFrom) == ""`; `resolveWorkflowFlowRef` bails early when `rs.restoredFrom != ""`.

## 8. Validation

- `V-1` Red-first TDD: both new tests fail on pre-fix HEAD (`git stash` isolating only the 3 fix files, new test file kept in place) — `manifest turnCount = <nil>, want 4` for Codex/Claude/Grok, and `restored run resolved flowRef` — then pass after. Assertions read raw manifest JSON / `resolveWorkflowFlowRef` return, so they compile on the baseline.
- `V-2` `TestSyncRestoreCarriesTurnCountAllProviders` — cross-provider parity: sync a flow-hub session with `TurnCount=4`, restore, assert the uploaded manifest carries `turnCount:4` AND the restored session's `TurnCount==4`, for Codex, Claude, and Grok.
- `V-3` `TestResolveWorkflowFlowRefSkipsRestoredRun` — a restored run (`restoredFrom="google_drive"`, `turnCount==0`, `workflowID` set) does NOT resolve its flowRef, while an identical **non-restored** control run still does (guard is scoped to restored runs, not broken across the board).
- `V-4` `go build ./...`, `go vet ./internal/runner/...`: clean. Full `go test ./...`: **18 failures, byte-identical to the pre-fix baseline** (`git stash` comparison — all machine/env/CLI/live-dependent: codex-CLI resume, grok live-detect, skills fixture, git-shim, Windows paths, a TempDir cleanup race), with the new BUG-315 tests flipping red→green. `TestRun20332FlowHubHistoryParityForEveryProvider/grok` is a known order-dependent flake (passes in isolation; absent from both full-sweep baseline and fix) — not caused by this change.
- `V-5` Live end-to-end on the running dev runner (fixed binary, project Gate-sandbox): re-synced flow hub `run-46797`, deleted it locally, restored from Drive → restored `turn_count=1` (**not 0** — round-tripped through the real Drive), resumed, sent a plain follow-up → **no new flow child spawned** (child count stayed 8), the hub took its own turn (turn-log wrote the follow-up prompt + a `codex_session` for the hub, `turn_count` advanced 1→2, status `completed`). Pre-fix this exact sequence restored to `turn_count=0` and re-spawned a coder.

## 9. Regression Guard

- tests: `apps/local-runner/internal/runner/bug315_restore_turncount_no_reflow_test.go` (`TestSyncRestoreCarriesTurnCountAllProviders`, `TestResolveWorkflowFlowRefSkipsRestoredRun`).
- alerts: none.
- audit checks: [CA-409](../../change-audit/CA-409-sync-manifest-carries-turncount-no-reflow-on-restore.md).

## 10. Follow-Up Document Updates

- upstream docs updated: [CP-51 Phase A/B Timeline And Verification Log](../../07-Coding-Plan/inprogress/CP-51-PhaseAB-Timeline-And-Verification-Log.md) — C6 restore-side note + the §6 gaps list updated in the same pass (C6 restore marked succeeded, BUG-315 recorded as the follow-up it uncovered).
- notes left unchanged on purpose: chats synced by a pre-fix manifest still restore with `turnCount==0`, but the `restoredFrom` guard prevents the re-trigger for them too (they simply have the less-precise count until re-synced). The already-restored 3 hubs are now past their first post-restore turn (`turnCount>=1`), so they no longer re-trigger regardless.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-315
change_type: bugfix
summary: chat-sync manifest now carries TurnCount and a restoredFrom guard blocks first-turn flow start on restored runs, so a Drive-restored flow chat's plain follow-up reaches the hub instead of re-spawning the entire flow.
# --->8---
