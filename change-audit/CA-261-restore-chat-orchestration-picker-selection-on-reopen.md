# CA-261: Restore Chat Orchestration Picker Selection On Reopen

## Scope

Fixed BUG-263, found live during CP-36 Scenario 11 testing (`run-11120`): reopening a Chat Mode Review Loop run after a runner restart showed the Chat Intent panel reset to "Normal" instead of the "Bug" / Review Loop selection the run was actually started with. `subMode`/`flowRef` were never persisted anywhere beyond the transient turn request, so `openHistoryRun`'s reopen logic (which already restores `chatMode`/`launchMode` per BUG-170) had nothing to restore them from.

## Changes

- `interactive_service.go`: `interactiveRun` gained `chatSubMode`/`chatFlowRef`, set in `startTurn`'s first-turn flowRef branch and included in `sessionStateOf`.
- `workflow_store.go`: `ProviderSessionState` gained `ChatSubMode`/`ChatFlowRef`.
- `local_file_session_store.go`: NDJSON record + both mapping directions carry the new fields.
- `interactive_resume.go`: `reconstructRun` restores them after a restart.
- `interactive_handlers.go`: `runHistoryItem` exposes `SubMode`/`FlowRef` from both the in-memory and persisted-session run-history sources.
- `contract.ts`: `RunHistoryItem.subMode`/`flowRef`.
- `store.ts`: `openHistoryRun` restores `chatStartMode`/`flowRef` from the history item alongside the existing `chatMode`/`launchMode` restoration.

## Verification

- `go build ./...`, `go vet ./internal/runner/` — passed.
- `npm --prefix apps/desktop-flowpilot run typecheck` — passed.
- `go test ./internal/runner/ -run 'TestLocalFileSessionStoreChatOrchestrationSelectionRoundTrip|TestChatModeRunHistoryExposesOrchestrationPickerSelection' -v -count=1` — passed.
- `go test ./internal/runner/... -count=1` — 1147 passed (2 more than the pre-fix baseline), 15 failed (pre-existing, environment-specific, unrelated), 14 skipped.
- New `store.test.ts` cases verified via `npm run typecheck` only — direct `tsx --test` execution is blocked by this repo's pre-existing unresolved `@/` Vite-alias limitation (same as noted in CA-251 for `AgentsPanel.test.ts`).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-263
change_type: bugfix
summary: persist and restore the Chat-Mode orchestration picker's subMode/flowRef selection so reopening a run after a restart shows the correct Bug/Review Loop state instead of resetting to Normal
# --->8---
