# BUG-265: Built-In Orchestration Select Missing When ChatStartMode Is Restored, Not Clicked

## Metadata

- Document ID: `BUG-265`
- Title: `Built-In Orchestration Select Missing When ChatStartMode Is Restored, Not Clicked`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [CP-36: Agent Review Loop And Main Hub Orchestration](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) (Scenario 11)
- Child Documents: `none`
- Related Documents: [BUG-263: Chat Mode Orchestration Picker Selection Lost On Reopen After Restart](./BUG-263-Chat-Mode-Orchestration-Picker-Selection-Lost-On-Reopen-After-Restart.md) (the fix that first made `chatStartMode` reach `"bugfix"` via a path other than the Bug tab's own click handler, surfacing this gap), [CA-263](../../../change-audit/CA-263-load-builtin-orchestration-options-whenever-bugfix-active.md)
- Replaces: `none`
- Tags: `ui, chat-mode, agent-flow-engine, regression`

## AI Quick View

### Summary

- Found live immediately after verifying BUG-263's fix: on first opening the app's chat view with the Bug tab already showing selected, the **Built-in orchestration** select was missing entirely — not showing "None", not showing "Review Loop", just absent. Clicking **New run** → **Bug** made it reappear (showing "None"). Reopening the same history chat afterward correctly showed "Review Loop" selected.
- Root cause: the select only renders when `builtinOrchestrationOptions.length > 0` (`ChatWorkspace.tsx`), and that list is only ever populated as a side effect of `setChatStartMode` — the function bound to the Bug tab's `onClick`. Any other path that sets `chatStartMode` to `"bugfix"` (BUG-263's `openHistoryRun` restore being the first one to exist) never calls the fetch, leaving the options list empty and the select invisible even though `chatStartMode`/`flowRef` were both correctly restored underneath.
- Confirms the user's own read of the situation exactly: the underlying data (`chatStartMode`/`flowRef`) was already correct (BUG-263's fix worked); only the UI's decision to *show* the select for that already-correct state was missing on the very first render.

### Current Ask

- The Built-in orchestration select must appear whenever `chatStartMode === "bugfix"` is active, regardless of whether that state was reached by clicking the Bug tab or by any other path (e.g. restoring a reopened run).

### Key Decisions

- `V-1` Fix at the render/data-loading boundary, not by chasing down every individual path that can set `chatStartMode`: add a `useEffect` in `ChatStartIntentPanel` that loads `builtinOrchestrationOptions` whenever `chatStartMode === "bugfix"` and the list is still empty. This covers BUG-263's `openHistoryRun` path today and any future path that sets `chatStartMode` directly, without needing a matching fix at each call site.
- `V-2` Left `setChatStartMode`'s own explicit `loadBuiltinOrchestrationOptions` call in place (harmless double-trigger guarded by the `length === 0` check in the new effect; the explicit call still fires immediately on a user click without waiting for a render+effect cycle, keeping the existing UI feel unchanged for that path).

### Constraints

- No change to how the built-in orchestration list is fetched (`GET /client/chat/builtin-orchestration-options?subMode=bug`) — this is purely about *when* the fetch is triggered on the client.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx`: `ChatStartIntentPanel` (~line 173), the new `useEffect` (~line 184), and the select's existing render gate (~line 246, `chatStartMode === "bugfix" && builtinOrchestrationOptions.length > 0`).
- `apps/desktop-flowpilot/src/state/store.ts`: `setChatStartMode` (~line 921, the only pre-existing trigger), `loadBuiltinOrchestrationOptions` (~line 946).
- Live evidence: user screenshots — (1) first chat open, Bug tab active, no Built-in orchestration select at all; (2) New run → Bug, select appears showing "None"; (3) reopening the earlier history chat, select correctly shows "Review Loop".

## 1. Issue Summary

Immediately after confirming BUG-263's fix restored `chatStartMode`/`flowRef` correctly on reopening a Chat-Mode Review Loop run, the user found a related display gap: on first opening the chat view with Bug already the active tab, the Built-in orchestration select itself didn't render at all, even though the underlying restored data was correct.

## 2. Parent Links

- impacted coding plan: [CP-36](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), Scenario 11 — found immediately after re-verifying this session's BUG-263 fix.

## 3. Environment and Reproduction

- environment: Desktop app, Chat Mode.
- reproduction steps:
  1. Reach a state where `chatStartMode` becomes `"bugfix"` without the user clicking the Bug tab themselves (e.g. reopening a run that was started via the Bug sub-mode picker, per BUG-263's restore path).
  2. Observe the Chat Intent panel: the Bug tab shows active/selected, but the Built-in orchestration select is entirely absent.
- frequency: deterministic for any `chatStartMode = "bugfix"` transition that doesn't go through the Bug tab's own `onClick`.

## 4. Expected vs Actual

- expected: the Built-in orchestration select renders (showing "None" or the restored flow's label) whenever Bug is the active intent, regardless of how it became active.
- actual: the select rendered only when `builtinOrchestrationOptions` had already been fetched, which only ever happened via the Bug tab's click handler — any other path left the list empty and the select invisible.

## 5. Impact

- users affected: anyone reopening a Chat-Mode Review Loop run (BUG-263's scenario), or any future flow that sets `chatStartMode` to `"bugfix"` without a direct Bug-tab click.
- workflows affected: display only — `flowRef` itself was already correctly restored and functionally in effect; only the select's visibility was wrong.
- severity: low-medium — confusing UI (looks like the picker selection was lost, even though it wasn't), directly adjacent to and easily mistaken for BUG-263 itself.

## 6. Root Cause

- confirmed cause: `ChatWorkspace.tsx`'s Built-in orchestration select is gated on `builtinOrchestrationOptions.length > 0`, and the only code path that ever populated `builtinOrchestrationOptions` was `setChatStartMode`'s own body (`if (mode === "bugfix") { void get().loadBuiltinOrchestrationOptions("bug"); }`) — a side effect tied to the Bug tab's `onClick`, not to the `chatStartMode` value itself. BUG-263's `openHistoryRun` fix sets `chatStartMode: "bugfix"` directly via `set(...)`, bypassing `setChatStartMode` entirely, so it never triggered the options fetch.
- evidence: the three-screenshot sequence — no select on first natural render with Bug already active, select appears (with "None") only after an explicit Bug-tab click via "New run", and correctly shows "Review Loop" once `builtinOrchestrationOptions` had already been populated by that intervening click before reopening the history chat.

## 7. Fix Strategy

- `F-1` `ChatWorkspace.tsx`: added a `useEffect` in `ChatStartIntentPanel` that calls `loadBuiltinOrchestrationOptions("bug")` whenever `chatStartMode === "bugfix"` and `builtinOrchestrationOptions.length === 0`, independent of how `chatStartMode` reached that value.

## 8. Validation

- `V-1` `npm --prefix apps/desktop-flowpilot run typecheck` — clean.
- `V-2` Not unit tested — this app has no React component test harness (no `@testing-library/react`/jsdom dependency, no existing test file for `ChatWorkspace.tsx` or any other component in this directory), so a rendering-level regression test could not be added without introducing a new test framework, judged out of scope for this fix. Verified by code review only: the new effect's dependency array (`chatStartMode`, `builtinOrchestrationOptions.length`, the stable store action reference) re-runs exactly when needed and is idempotent (guarded by `.length === 0`, so it won't refetch once options are loaded).
- `V-3` Not performed: a live re-test against the exact reported repro (reopen a Bug/Review-Loop-started run in a fresh app session and confirm the select now appears immediately).

## 9. Regression Guard

- tests: none — see `V-2`.
- audit checks: `gitnexus_detect_changes()` was not run — GitNexus MCP tools were unavailable in this thread.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: `setChatStartMode`'s own explicit `loadBuiltinOrchestrationOptions` call was left in place rather than removed in favor of the new effect alone, to keep the existing immediate-fetch-on-click feel for the common interactive path.
