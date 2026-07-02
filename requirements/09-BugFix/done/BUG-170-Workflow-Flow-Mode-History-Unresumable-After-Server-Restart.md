# BUG-170: Workflow / Flow-Mode History Unresumable After Server Restart

## Metadata

- Document ID: `BUG-170`
- Title: `Workflow / Flow-Mode History Unresumable After Server Restart`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `none`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-112` (multi-turn transcript replay), chat-session resume work (T-7)
- Replaces: `none`
- Tags: `agent-flow-engine, runner, desktop, resume, history, flow-mode, known-limitation`

## AI Quick View

### Summary

- After the runner server is restarted, only NORMAL chat history runs can be reopened. Opening a workflow / flow-mode run from history fails.
- Root cause is an explicit guard: after a restart the run is not in memory, so `resumeRun` falls back to `loadPersistedRun`, which hard-rejects any run whose `RunKind != "chat"` with a `409 resume_unsupported` ("only chat runs can be resumed in this version").
- Secondary desktop gap: even if resume returned, `openHistoryRun` never restores `chatMode` / `launchMode` / `workflowId`, so the UI would stay in whatever mode it was in and the Flow Mode surfaces (step sidebar, agents) would not come back.

### Current Ask

- Decide and implement the intended post-restart behavior for workflow/flow-mode runs: at minimum make them **viewable read-only**; ideally make them **resumable**. Restore the correct chat/launch mode on reopen either way.
- Resolved: implemented full resumability, not just read-only viewing — the existing reconstruction path already generalized to workflow runs once the blanket rejection was removed.

### Key Decisions

- `V-1` **(done)** A workflow/flow-mode history run must not return a bare `409 resume_unsupported` that reads to the user as "history is broken." Implemented as full resume, exceeding the minimum read-only bar.
- `V-2` **(done)** `openHistoryRun` now sets `chatMode` (and `launchMode` / `selectedWorkflowId` where applicable) from the reopened run's kind so the UI renders in the right mode.

### Constraints

- Turned out not to require reconstructing orchestrator/agent-graph/step-runtime state from scratch: `reconstructRun` and `ensureResumeReady` were already generic over `runKind` (restoring `workflowID`, flow loop state, active flow edges/nodes regardless of kind) — the only kind-specific code was the guard itself plus one unconditional chat-step seed call, both addressed in Fix Strategy.
- Must not regress the existing chat-run resume path (BUG-112 multi-turn replay, cross-account resume tests) — verified via the full `go test ./...` run.

### Open Questions

- None — resolved. `reconstructRun` already restores `runKind`, `workflowID`, flow loop state (`AutoOrchestrate`, `FlowCohortID`, `ActiveFlowEdges`/`ActiveFlowNodes`) and `ensureResumeReady` is provider-generic (no `runKind` branching at all), so the persisted state already sufficed for full resume — no persistence extension was needed. The `RunKind != "chat"` guard was the only thing standing in the way (F-2 below), not a genuine data gap.

### Source Refs

- `apps/local-runner/internal/runner/interactive_resume.go:1114` (`loadPersistedRun`)
- `apps/local-runner/internal/runner/interactive_resume.go:1129-1132` (the `RunKind != "chat"` → `409 resume_unsupported` guard)
- `apps/local-runner/internal/runner/interactive_handlers.go:700` (`resumeRun` — in-memory-first, then `loadPersistedRun`)
- `apps/local-runner/internal/runner/interactive_handlers.go:734` (chat-only `StepID` synthesis on resume)
- `apps/desktop-flowpilot/src/state/store.ts:1372` (`openHistoryRun` — never sets `chatMode`/`launchMode`)
- `apps/desktop-flowpilot/src/types/contract.ts:322` (`RunHistoryItem`, `runKind`, `workflowId`)

## 1. Issue Summary

Restart the runner, then try to open a previous chat from history. Normal chat runs open fine. Workflow / flow-mode runs cannot be opened — the runner returns `resume_unsupported`. Effectively, after a restart history access is limited to normal chat mode.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop-flowpilot + local runner; a workflow/flow-mode run recorded in history.
- reproduction steps:
  1. Run a workflow / flow-mode session so it appears in history.
  2. Restart the runner server (clears in-memory `s.runs`).
  3. In the desktop, open that workflow run from history.
  4. Observe it fails to open (runner logs `[chat-history-open] persisted run unsupported ... run_kind="workflow"`, returns `409 resume_unsupported`). A normal chat run opened the same way works.
- frequency: deterministic for non-chat runs after a restart.

## 4. Expected vs Actual

- expected: workflow/flow-mode history runs can be reopened after a restart (at least to view the transcript, ideally to continue), in their correct mode.
- actual: only `runKind == "chat"` runs reopen; workflow runs return `409 resume_unsupported`.

## 5. Impact

- users affected: anyone who restarts the runner and wants to revisit a workflow/flow-mode run.
- workflows affected: all workflow / flow-mode history access across restarts.
- severity: medium-high — data is persisted but inaccessible; the history entry is present but dead, which reads as data loss.

## 6. Root Cause

- confirmed cause (runner): `resumeRun` (`interactive_handlers.go:700`) looks up `s.runs[runID]`; after a restart that map is empty, so it calls `loadPersistedRun` (`interactive_resume.go:1114`). That function explicitly returns `newAPIErr(http.StatusConflict, "resume_unsupported", "only chat runs can be resumed in this version")` whenever `st.RunKind != "chat"` (`:1129-1132`). Workflow runs have `runKind` `""`/`"workflow"`, so they are always rejected post-restart. This is a deliberate, documented limitation rather than an accidental break.
- contributing cause (desktop): `openHistoryRun` (`store.ts:1372`) sets `runId`, `mainRunId`, `status`, `activeStepId`, etc., but never sets `chatMode` / `launchMode` / `workflowId`. So even if the runner returned a handle, the desktop would treat the reopened run under the current mode, and Flow Mode surfaces (the step sidebar from BUG-168, the agents panel) would not re-engage.
- evidence: runner log `[chat-history-open] persisted run unsupported run_id=%q run_kind=%q`; `RunHistoryItem.runKind` doc comment `"chat" for normal_chat runs; undefined for workflow/step runs` (`contract.ts:332`).

## 7. Fix Strategy

Went with full resumability (F-2 tier) since the plumbing already supported it:

- `F-1` **(implemented)** Removed the blanket `st.RunKind != "chat"` → `409 resume_unsupported` guard in `loadPersistedRun` (`interactive_resume.go`). `ensureResumeReady` was already provider-generic and `reconstructRun` already restored `runKind`/`workflowID`/flow loop state regardless of kind, so lifting the guard was sufficient for full resume, not just read-only viewing.
- `F-1b` **(implemented, safety fix)** `reconstructRun`'s synthetic-chat-step seed call (`seeder.seed(rs.id, []RuntimeWorkflowStep{{ID: "chat-"+rs.id, StepType: "chat", ...}})`) used to run unconditionally. It was only ever reached for chat runs before this fix (the guard above blocked everything else), so it was implicitly chat-shaped. Now that workflow runs reach `reconstructRun` too, this call is gated on `st.RunKind == "chat"` — mirroring `createRun`'s own runKind branch — so a resumed workflow run's real step-runtime list isn't overwritten with a fake single "chat" step (`seed` replaces the store's per-run step list wholesale).
- `F-3` **(implemented)** `openHistoryRun` (`store.ts`) now derives `chatMode` from `historyItem.runKind` (`"workflow_step_auto"` unless `runKind === "chat"`) and, when a `workflowId` is present, also restores `launchMode: "workflow"` and `selectedWorkflowId`, so a reopened workflow run re-mounts its Flow Mode surfaces (step-timeline sidebar from BUG-168, Agents panel) instead of staying in whatever mode the UI was previously in.

## 8. Validation

- `V-1` Added `TestResumeRunReconstructsWorkflowRunFromDisk` (`cross_account_resume_test.go`), modeled on the existing `TestResumeRunReconstructsChatRunFromDisk` — a persisted `runKind: "workflow"` session resumes successfully post-restart (`apiErr == nil`), and confirms `LoadRunSteps` was NOT clobbered with a fake chat step (`F-1b`). Passes.
- `V-2` Replaced `TestResumeRunRestoredWorkflowRunUnsupportedForMVP` (which asserted the old `resume_unsupported` rejection) — that behavior was an intentional MVP-era cut, now superseded by this fix.
- `V-3` Ran the full `go test ./...` for the runner: two pre-existing, unrelated failures confirmed present on a clean stash (`TestSkillsMerge*WithPrecedence`, `TestStartInteractiveAuthLaunchesFromWorkspace` — OS path quoting), no new failures from this change. All 10 `TestResumeRun*` tests and the Drive-sync `TestBuildChatSessionSyncManifestRejectsNonChatRun` (a separate, intentionally-untouched guard) pass.
- `V-4` `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- `V-5` Not executed: an actual runner-restart + reopen through the desktop UI, since no runner + provider account is available in this environment. The user should confirm a real workflow/flow-mode history item reopens (and shows the Flow Mode sidebar/agents) after a restart on next use.

## 9. Regression Guard

- tests: `TestResumeRunReconstructsWorkflowRunFromDisk` (new, runner) guards the resume-succeeds + no-step-corruption behavior; existing chat-run resume tests (`TestResumeRunReconstructsChatRunFromDisk`, `TestResumeRunReconstructsChatRunSeedsChatStep`) guard no regression to the chat path.
- alerts: none.
- audit checks: recorded in `change-audit/CA-207-workflow-flow-mode-resume-after-restart.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none — this restores intended behavior within the existing CP-36 resume design rather than introducing new semantics.
- notes left unchanged on purpose: `chat_session_sync.go`'s own, separate `RunKind != "chat"` guard (Google Drive chat sync — `BuildChatSessionSyncManifest`) is intentionally untouched; it scopes a different feature (which runs can sync to Drive) and its own test (`TestBuildChatSessionSyncManifestRejectsNonChatRun`) still passes unmodified.
