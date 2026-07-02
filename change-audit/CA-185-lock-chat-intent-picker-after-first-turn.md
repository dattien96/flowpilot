# CA-185: Lock Chat Intent Picker After the First Turn (BUG-NOTE-CP42 #29)

## Scope

Verified and fixed a P3 issue from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`: the Chat Intent / built-in orchestration picker became interactive again after the first turn completed, even though nothing it controls has any effect past that point.

## The bug

Tracing the full path revealed this is a UI-only drift, not a wire-level one: `store.ts`'s `sendMessage` already correctly gates `changeType`/`sourceDocId`/`subMode`/`flowRef` on `isFirstChatTurn = chatMode === "normal_chat" && !runId` — once a `runId` exists (set on the very first send and never cleared for the life of that chat), the client itself permanently stops transmitting these fields, matching the runner's own `turnCount == 0` gate exactly. So no bad data actually reaches the backend after turn 1.

The bug is that `ChatStartIntentPanel` (`ChatWorkspace.tsx`) only disabled its controls while `runStatus === "running"`. The moment the first turn finished, the picker became clickable again — misleadingly suggesting to the user that changing the Task/Bug tab, the tracked doc ID, or the built-in orchestration flowRef mid-conversation does something, when the client has already permanently stopped sending it.

## Fix

Added `chatStarted = Boolean(runId)` and changed the disable condition to `runStatus === "running" || chatStarted`, so the picker locks the moment a `runId` exists and stays locked for the rest of that chat's life — not just while a turn is actively in flight. Updated the panel's help copy to explain why: "Locked after the first message — start a new chat to change the intent." (instead of the old "Disabled while the AI is running," which was now inaccurate).

## Verification

- `tsc` against the **real project config** (`apps/desktop-flowpilot/tsconfig.json`) instead of the narrower `tsconfig.phase1-tests.json` used for the earlier fixes in this pass — confirmed via `--listFiles` that this config actually includes `.tsx` files (`ChatWorkspace.tsx`, `WorkflowsSettings.tsx`), which `tsconfig.phase1-tests.json` does not. Clean compile, zero errors. This is a stronger verification than what CA-182/183/184 had available at the time; re-running it now also retroactively confirms those three fixes compile cleanly under the real app config, not just the narrower phase1-tests one.
- Not verified in a live browser preview (see CA-184's note — `desktop-flowpilot` is an Electron app, not reachable through this session's web-preview harness).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: lock the Chat Intent / built-in orchestration picker once a runId exists (not just while a turn is running), since the client already permanently stops sending changeType/subMode/flowRef past the first turn and the picker's re-enabling was purely misleading UI drift
# --->8---
