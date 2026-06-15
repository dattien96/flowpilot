# BUG-060: Desktop Run History Empties After Switching Runs

## Metadata

- Document ID: `BUG-060`
- Title: `Desktop Run History Empties After Switching Runs`
- Phase: `bugfix`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md), [04: Detailed Coding Plan](../../10-Refactor/New-System/04-Detailed-Coding-Plan.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: `none`
- Related Documents: [BUG-052: Desktop Stale Stream Events Cross Runs](../done/BUG-052-Desktop-Stale-Stream-Events-Cross-Runs.md), [Task-044: Desktop Chat Mode Split And Provider Controls](../../08-Task/todo/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md), [08: Desktop Chat New Plan](../../10-Refactor/New-System/08-Desktop-Chat-New-Plan.md), [CA-020: Workflow UI Chat Feed and Continue Flow](../../change-audit/CA-020-workflow-ui-chat-feed-and-continue-flow.md), [CA-021: Workflow Chat Runtime and Font Tuning](../../change-audit/CA-021-workflow-chat-runtime-and-font-tuning.md)
- Replaces: `none`
- Tags: `desktop, run-history, workflow-runs, persistence, runner, race-condition, regression`

## AI Quick View

### Summary

- Starting two runs and switching back and forth between them eventually makes the desktop History panel show empty ("No runs for this project"), even though runs exist.
- The runner serves run history exclusively from an in-memory map (`InteractiveService.s.runs`) that is populated only by `startRun` and is never rehydrated from the persisted session store — so any process restart or app-server/active-account recreation drops history.
- Provider-session state IS persisted separately (`persistProviderSession` → `UpsertProviderSession`), but `projectRunHistory` never reads it back; persistence is additionally a no-op when `workflowStore` is the fake store.
- Desktop `loadRunHistory` has no stale-response/concurrency guard and silently falls back to the empty baseline on error, which can mask or compound the empty result.

### Current Ask

- Root-cause and plan only this pass. Do NOT implement the code fix yet. Confirm the runtime trigger, then record the fix strategy.
- A deterministic repro test is added in this pass (`TestRunHistoryEmptiesAfterServiceRecreation` in `interactive_service_test.go`) to confirm the in-memory-only root cause via runner restart simulation — no fix yet.

### Key Decisions

- `V-1` Run history must survive a runner / app-server / active-account recreation within the same machine, since session state is already persisted.
- `V-2` The history read path must source from the persisted session store (rehydrate or query), not solely the in-memory `s.runs` map.
- `V-3` Desktop `loadRunHistory` must ignore stale/superseded responses and must not silently present an empty list when a fetch errored.

### Constraints

- Source is an unverified note in `08-Desktop-Chat-New-Plan.md` line 14; the in-memory-only read path is confirmed by code, but the exact "switch back and forth" trigger is a hypothesis pending runtime confirmation.
- Ties into the `08` note that the `workflow_provider_sessions` table / rehydration path may be missing; a full fix may require a persisted history read endpoint.
- GitNexus MCP tools were not exposed in this thread; no application symbols were edited. Run impact analysis before editing `projectRunHistory`, `loadRunHistory`, or `InteractiveService` run state.

### Open Questions

- Which concrete action during "switch back and forth" empties the in-memory map at runtime: (a) app-server/active-account recreation per the `04` "recreate on active-account change" decision, (b) a dev-mode `go run` runner restart, (c) a desktop-side `loadRunHistory` race/error, or a combination?
- Should history be rehydrated into `s.runs` on startup, or should `projectRunHistory` query the persisted store directly (read-through)?
- Should normal-chat runs (Task-044 `T-7`) share this same history path, which makes fixing this a prerequisite for that task?

### Source Refs

- `08-Desktop-Chat-New-Plan.md:14` — original note ("start 2 run. swith qua lai 1 hoi, toi 1 luc press History se empty show").
- `apps/local-runner/internal/runner/interactive_handlers.go:542` — `projectRunHistory` reads `s.runs` only.
- `apps/local-runner/internal/runner/interactive_service.go:44,196,219` — in-memory `runs` map; `persistProviderSession` writes a separate store.
- `apps/desktop-flowpilot/src/state/store.ts:348` — `loadRunHistory` (no stale guard).
- `apps/desktop-flowpilot/src/components/RunStatus.tsx:89` — renders "No runs for this project" when `runHistory` is empty.

## 1. Issue Summary

On the desktop FlowPilot app, after starting two workflow runs and switching back and forth between them (via the History panel) for a while, opening the History panel at some point shows an empty list ("No runs for this project"), even though the runs were started and should appear. The behavior is intermittent ("at some point"), not on every switch.

## 2. Parent Links

- impacted coding plan: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md), [04: Detailed Coding Plan](../../10-Refactor/New-System/04-Detailed-Coding-Plan.md)
- impacted tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: desktop-flowpilot (Electron renderer) against the local Go runner (`apps/local-runner`), `HttpWsRunnerClient` transport.
- reproduction steps:
  1. Select a project.
  2. Start run A (send a prompt), then start run B (New run → send another prompt), so two runs exist.
  3. Open History, switch to run A; open History, switch to run B; repeat several times.
  4. Open History again.
- frequency: intermittent ("at some point"); not reproduced deterministically yet. Needs a controlled repro to confirm the exact trigger.

## 4. Expected vs Actual

- expected: the History panel lists all runs for the selected project regardless of how many times the user switched between runs, and survives runner/app-server recreation since session state is persisted.
- actual: after switching back and forth, the History panel renders "No runs for this project" (empty list).

## 5. Impact

- users affected: desktop users running multiple runs per project.
- workflows affected: run history / resume-from-history; also blocks Task-044 normal-chat history (`T-7`) if it reuses this path.
- severity: medium — no data corruption, but users lose the ability to find/resume prior runs; erodes trust in the History feature.

## 6. Root Cause

- hypothesis (runtime trigger, unconfirmed): "switch back and forth" empties the in-memory run map via one of — (a) app-server/active-account recreation (the `04` Resolved Decision recreates the shared app-server on active-account change), (b) a dev `go run` runner restart between switches, or (c) a desktop-side `loadRunHistory` race/error that overwrites or retains the empty baseline. The intermittency ("at some point") fits an event-triggered reset rather than per-switch behavior.

- confirmed cause (by code inspection):
  1. The runner serves history from an **in-memory, process-scoped** map. `projectRunHistory` (`interactive_handlers.go:542`) iterates `s.runs`; that map is initialized empty (`interactive_service.go:196`) and populated only by `startRun` (`interactive_handlers.go:476`). There is **no rehydration** of `s.runs` from any persisted store on startup, and `projectRunHistory` never reads the persisted store. Therefore any restart/recreation of the `InteractiveService` empties history.
  2. Session state IS persisted on a **separate** path — `persistProviderSession` → `InteractiveStateStore.UpsertProviderSession` (`interactive_service.go:219`) — but the history read path ignores it. When `workflowStore` does not implement `InteractiveStateStore` (e.g. the default fake store, `interactive_service.go:186`), persistence is a **no-op** (`return nil`), so nothing survives at all.
  3. Desktop `loadRunHistory` (`store.ts:348`) has **no request-sequencing / stale-response guard**; overlapping calls (e.g. `toggleRunHistory` + the `sendPrompt` `finally` refetch when `historyOpen`) let a slower/older response win. On fetch error it leaves `runHistory` at its previous value, which is the empty baseline right after `selectProject` (`store.ts:201` sets `runHistory: []`) — rendered as "No runs for this project."

- evidence: code locations above. Note the sibling defect class already documented in [BUG-052](../done/BUG-052-Desktop-Stale-Stream-Events-Cross-Runs.md) (stale events crossing runs) — same "multiple runs + switching" surface.

## 7. Fix Strategy

- `F-1` Make the runner history read path source from persisted session state: either rehydrate `s.runs` from `InteractiveStateStore` on startup, or have `projectRunHistory` read-through to a `ListProviderSessions(projectId)` query. Decide per Open Question 2.
- `F-2` Ensure a real persisted store backs `workflowStore` in the desktop/runner wiring path (not the fake store), so `persistProviderSession` is not a silent no-op; if the fake store is intentional for some modes, surface that history is non-persistent rather than showing empty.
- `F-3` Add a stale-response guard to desktop `loadRunHistory` (request token / latest-wins) so an older or errored response cannot overwrite a newer successful list.
- `F-4` On `loadRunHistory` error, do not present a misleading empty list — distinguish "no runs" from "failed to load" in `RunStatus.tsx` (error state vs empty state).
- `F-5` Confirm/repair the `workflow_provider_sessions` persistence + read contract referenced in the `08` plan note if the rehydration path requires it.

## 8. Validation

- `V-1` **Repro test (added this pass, confirmed failing):** `TestRunHistoryEmptiesAfterServiceRecreation` in `apps/local-runner/internal/runner/bug060_test.go` — starts 2 runs on one `InteractiveService`, recreates the service with the same `workflowStore` (simulating a runner/app-server restart), calls `GET /client/projects/proj-web/workflow-runs`, asserts 2 items returned. **Result: `len=0, want 2` — bug confirmed.** Pre-restart phase (2 items visible on the original service) passes, so the test setup is correct. Test must be updated to pass when `F-1`/`F-2` are implemented.
- `V-2` Runner HTTP test: after `F-1` rehydration — persist sessions, recreate `InteractiveService`, confirm `GET /client/projects/{projectId}/workflow-runs` returns the persisted runs (not empty).
- `V-3` Desktop test: overlapping `loadRunHistory` calls resolve latest-wins; an errored fetch shows an error state, not an empty list.
- `V-4` Manual: switch between 2 runs repeatedly; History remains populated throughout.

## 9. Regression Guard

- tests: add the runner rehydration test (`V-2`) and the desktop race/error test (`V-3`) to the suite so the in-memory-only regression cannot return.
- alerts: none.
- audit checks: add a `change-audit/CA-*` entry when the fix lands (per audit-logging skill).

## 10. Follow-Up Document Updates

- upstream docs that must change: if history becomes a persisted read contract, update `SS-11` (workflow-run vs AI-session boundary), `SD-12`, and the `04` shared contract to document the persisted history read path; resolve the `08` note about the `workflow_provider_sessions` table.
- notes left unchanged on purpose: Task-044 `T-7` (normal-chat history) stays a separate decision but should reference this bug as a prerequisite if it reuses the same history path.
