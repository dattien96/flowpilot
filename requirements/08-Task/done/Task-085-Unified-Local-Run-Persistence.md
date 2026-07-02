# Task-085: Unified Local Run Persistence

## Metadata

- Document ID: `Task-085`
- Title: `Unified Local Run Persistence (Chat + Flow On localFileSessionStore, Drive-Synced)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-19`
- Last Updated: `2026-06-29`
- Parent Documents: [CP-36: Generic Agent-Flow Engine And Review Loop](../../07-Coding-Plan/inprogress/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md)
- Child Documents: `None`
- Related Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/done/CP-19-Multiple-Agents.md), [Task-090: Bounded Flow Runtime Executor](./Task-090-Bounded-Flow-Runtime-Executor.md), [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- Replaces: `Supersedes the original Task-085 scope (Flow-Mode Supabase Agent Runs And Message Bus)`
- Tags: `multi-agent, flow-mode, local-persistence, drive-sync, resume, deprecation, go`

## AI Quick View

### Summary

- **Supersedes the original Supabase-agent-runs plan.** Per CP-36 `P-5`/`Q-5`, run data for **both** chat and flow modes persists through the existing `localFileSessionStore` (`sessions.ndjson`) and syncs cross-PC via Drive (already shipped). No `agent_runs`/`agent_messages` Supabase tables are created.
- Extend the session-snapshot serializer + resume path so the new flow fields (`mode`, `round`, `cap`, `activeNode`, `extendCount`, `autoOrchestrate`, `flowCohortId`, edge/cohort state) survive a runner restart and a cross-PC move (closes CP-36 **G1**).
- Mark the legacy `SupabaseWorkflowStore` **run** path deprecated and unwired in production (already true at `cli/root.go:120-123`); the `workflow_run_logs`/`workflow_run_sessions`/`workflow_run_steps` tables are ignored, not migrated. Flow/Step **definitions** stay on Supabase.

### Current Ask

- Make the local file store the explicit, sole run sink for both modes; persist + restore flow loop state; deprecate the Supabase run path; and verify Drive sync + the desktop HTTP read path — with restart/resume/round-trip tests.

### Key Decisions

- `T-1` One run sink: `localFileSessionStore` for chat **and** flow. The `WorkflowStore` injected in `cli/root.go` is the local store; `SupabaseWorkflowStore` run methods are deprecated (type retained for compile/back-compat, not wired).
- `T-2` Flow run state rides in the **session record** (the agent sub-graph: spawn tree, rounds, bus, loop state), not in the in-memory-only `fakeWorkflowStore` step methods.
- `T-3` Definitions are unchanged on Supabase; only **run** data moves/stays local.
- `T-4` Cross-PC liveness is **Drive cadence, not realtime** — accepted (CP-36 `Q-5`).

### Constraints

- Run GitNexus impact analysis before editing `interactive_service.go` serializer, `local_file_session_store.go`, `interactive_resume.go`, `chat_session_sync.go`; warn on HIGH/CRITICAL.
- Do not regress chat persistence, session resume (SD-14), or Drive sync.
- No new Supabase run migration; do not drop legacy tables in this task (deprecate only).
- Preserve YOLO=false gate behavior on resume; replay must not auto-approve/auto-answer.

### Open Questions

- None blocking. If a future flow needs realtime cross-PC, that is a separate design (not this task).

### Source Refs

- CP-36 `P-5`, `Q-5`, `G1`; SD-19 `D-3` (definitions on Supabase, runs local in Chat tier); SD-14 (resume/home-sync).
- Anchors: `cli/root.go:120-123` (`NewLocalFileSessionStore` → `NewInteractiveServiceWithStore`); `local_file_session_store.go:16-37`; `supabase_workflow_store.go:28-30` (legacy run path); `interactive_service.go:744-762` (session snapshot serializer); `interactive_resume.go`; `chat_session_sync.go`; `apps/admin-web/.../http-local-runner-gateway.ts`.

## 1. Goal

All multi-agent/flow run state persists locally through `localFileSessionStore`, survives a runner restart and a Drive-synced cross-PC move, and the legacy Supabase run path is deprecated — while flow/step definitions continue to read from Supabase.

## 2. Parent Links

- coding plan: [CP-36](../../07-Coding-Plan/inprogress/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) `P-5`
- tech design: [SD-19](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) `D-3`; [SD-14](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- system spec: [SS-16](../../05-System-Specs/SS-16-Agent-Flow-Engine.md) `BR-3`
- specific upstream ids: CP-36 `P-5`, `Q-5`

## 3. Trigger

CP-36 unifies the engine across chat and flow, and the user has decided run data is local (Drive-synced) for both modes, retiring the Supabase run tables. The executor's loop state (Task-090) must survive restart/resume, which the current serializer does not cover.

## 4. Exact Change

- `T-1` Confirm + document that production injects `localFileSessionStore` as the `WorkflowStore` for both modes (`cli/root.go:120-123`). Add a guard/test that no production constructor wires `SupabaseWorkflowStore` for run data.
- `T-2` Mark `SupabaseWorkflowStore` run methods (`ApplyStepTransition`, `SetRunStatus`, `AppendLog`, `AppendEvent`) deprecated (doc comment + `// Deprecated:`); keep them compiling for back-compat; ensure no production path calls them.
- `T-3` Extend the session-snapshot serializer (`interactive_service.go:744-762`) to include flow fields: `mode`, `round`, `cap`, `activeNode`, `extendCount`, `autoOrchestrate`, `flowCohortId`, and the cohort/edge buffers needed to rebuild the in-flight graph.
- `T-4` Extend `interactive_resume.go` to restore those fields (mirror the existing `pendingAgentContext` restore) so a restarted/synced run resumes the loop at the right round/state.
- `T-5` Confirm `chat_session_sync.go` carries the extended manifest (the new fields ride in the same `sessions.ndjson` record already synced).
- `T-6` Verify the desktop reads run state via the local-runner HTTP gateway (`http-local-runner-gateway.ts`), not Supabase; note legacy admin-web `workflow_run_*` reads are out of scope (deprecated client).

## 5. Touched Areas

- files: `interactive_service.go` (serializer), `interactive_resume.go`, `local_file_session_store.go`, `chat_session_sync.go`, `cli/root.go` (confirm), `supabase_workflow_store.go` (deprecate run methods); tests `local_file_session_store_test.go`, `chat_session_sync_test.go`, `interactive_service_test.go`.
- modules: local-runner persistence + resume; Drive sync.
- routes: none new (desktop reads existing HTTP run-state endpoints).
- tables: none new. Legacy `workflow_run_logs`/`workflow_run_sessions`/`workflow_run_steps` deprecated (not dropped). Flow/Step definition tables unchanged.

## 6. Acceptance Check (Definition of Done)

- [ ] A flow run (coder + reviewers + loop state) **survives a runner restart** reconstructed purely from `sessions.ndjson`, with **no Supabase** call.
- [ ] Resume restores `mode`, `round`, `cap`, `activeNode`, `extendCount`, `autoOrchestrate`, `flowCohortId` to the pre-restart values.
- [ ] A Drive sync **round-trips a flow run** to a second machine; the run state (graph + loop) is present after sync.
- [ ] A guard/test asserts **no production path writes** `workflow_run_logs`/`workflow_run_sessions`/`workflow_run_steps`.
- [ ] Flow/Step **definition** reads still hit Supabase (unchanged).
- [ ] `SupabaseWorkflowStore` run methods are marked deprecated and have no production caller; the type still compiles.
- [ ] Existing single-agent chat runs persist + resume unchanged (regression).
- [ ] YOLO=false gates are preserved on resume (no auto-approve/auto-answer).

## 7. Out of Scope

- Creating `agent_runs`/`agent_messages` Supabase tables (cancelled scope); realtime cross-PC run streaming; dropping the legacy `workflow_run_*` tables; the executor logic (Task-090); the review tool/config (Task-091).

## 8. Completion Notes

- result:
- follow-ups: CP-19 should drop/redirect its reference to the original Task-085 Supabase scope (now superseded here).
- upstream docs updated:
