# Task-190: Sync And Restore Flow-Engine Runs To Google Drive

## Metadata

- Document ID: `Task-190`
- Title: `Sync And Restore Flow-Engine (Workflow) Runs To Google Drive`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-07`
- Last Updated: `2026-07-08`
- Parent Documents: [CP-36: Generic Agent-Flow Engine And Review Loop](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md)
- Child Documents: `None`
- Related Documents: [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md), [Task-085: Unified Local Run Persistence](../done/Task-085-Unified-Local-Run-Persistence.md), [Task-103: Engine Local Store And Drive Sync](../done/Task-103-Engine-Local-Store-And-Drive-Sync.md), [BUG-119: Chat Sync Excludes Child Agent Runs](../../09-BugFix/done/BUG-119-Chat-Sync-Excludes-Child-Agent-Runs.md), [BUG-123: Remote Restore Flattens And Loses Child Agent Chats](../../09-BugFix/done/BUG-123-Remote-Restore-Flattens-And-Loses-Child-Agent-Chats.md), [BUG-170: Workflow Flow-Mode History Unresumable After Server Restart](../../09-BugFix/done/BUG-170-Workflow-Flow-Mode-History-Unresumable-After-Server-Restart.md), [BUG-250: Restarted Flow Hub Run Permanently Unresumable — Placeholder Session](../../09-BugFix/done/BUG-250-Restarted-Flow-Hub-Run-Permanently-Unresumable-Placeholder-Session.md), [BUG-251: Restarted Child Agent Shows Permanently Stale Running Status](../../09-BugFix/done/BUG-251-Restarted-Child-Agent-Shows-Permanently-Stale-Running-Status.md), [BUG-254: Failed Cohort Reviewer Mark-Done And Hidden Node Identity](../../09-BugFix/done/BUG-254-Failed-Cohort-Reviewer-Mark-Done-And-Hidden-Node-Identity.md), [BUG-256: Restarted Flow Step Timeline Loses Reviewer Statuses When Nodes Share Agent](../../09-BugFix/done/BUG-256-Restarted-Flow-Step-Timeline-Loses-Reviewer-Statuses-When-Nodes-Share-Agent.md), [BUG-260: Resumed Flow Fast-Path Overwrites Failed Cohort Member As Done](../../09-BugFix/done/BUG-260-Resumed-Flow-Fast-Path-Overwrites-Failed-Cohort-Member-As-Done.md), [BUG-263: Chat-Mode Orchestration Picker Selection Lost On Reopen After Restart](../../09-BugFix/done/BUG-263-Chat-Mode-Orchestration-Picker-Selection-Lost-On-Reopen-After-Restart.md)
- Replaces: `None`
- Tags: `flow-engine, chat-sync, google-drive, workflow, restore, runner, desktop, cross-pc, regression-sensitive`

## AI Quick View

### Summary

- Today only `runKind == "chat"` runs can sync to Google Drive. Selecting a flow-engine / workflow run (`runKind` empty or `"workflow"`) and clicking **Sync** silently does nothing — the frontend filters the selection to an empty list and the confirm dialog never opens; the backend would reject it with `409 resume_unsupported` ("only chat runs can be resumed in this version") anyway, and a test (`TestBuildChatSessionSyncManifestRejectsNonChatRun`) currently pins that rejection.
- This executes CP-36 `P-5` ("run data for **both** chat and flow modes… Drive-synced… restored on resume") and unblocks CP-36 **Scenario 6 — Drive Sync Cross-PC**, the only CP-36 E2E scenario still unpassed. It also reconciles **DOD-5**, which is checked but over-claims flow-run cross-PC Drive sync that was never actually delivered.
- **Non-obvious hard part (BUG-250):** a flow **hub** run's `provider_session_id` stays the synthetic placeholder `"thread-<n>"` in *every* record — including the `completed` one — because CP-42 suppresses the hub's own provider turn. So a flow hub run has **no own transcript file to upload**; naively lifting the gate makes `BuildChatSessionSyncManifest` fail with `session_unavailable`. A flow run's content is its **flow runtime state + child transcripts**, not the hub's own rollout file.
- **Regression-sensitive:** the Drive restore path is a second "rebuild-from-external-source" path alongside the local restart-resume that BUG-250/251/254/256/260 hardened. It must apply the same placeholder-session bypass and status normalization, or it re-opens those exact bugs on the cross-PC path.

### Current Ask

- Make flow-engine / workflow runs syncable to Google Drive and restorable on another machine at parity with the local restart-resume path: flow runtime state + child agent transcripts round-trip, the board renders correct round/children/status, and none of the Scenario 5/9/11 bugfixes regress.

### Key Decisions

- `T-1` One syncable-run-kind rule (`isSyncableRunKind` in Go, `isSyncableRun` in TS) replaces scattered `runKind === "chat"` checks so chat and flow/workflow top-level runs share one definition.
- `T-2` Extend `ChatSessionSyncManifest` with the flow runtime fields persisted in `sessions.ndjson` (`LoopState`, `AutoOrchestrate`, `FlowCohortID`, `ActiveFlowEdges`, `ActiveFlowNodes`, `PendingAgentContext`) and round-trip them on restore so Machine B's board renders round/children/blocked state.
- `T-3` **A flow hub run syncs even with a placeholder / missing provider file.** The provider-transcript upload becomes optional for a run whose session id is still the synthetic placeholder (reuse the `skipsResumeSessionValidation` invariant from BUG-250); the run's substance travels as flow state + `ChildAgents` transcripts (the BUG-119 per-child upload path).
- `T-4` **Restore reuses the hardened resume invariants, not a parallel copy.** Drive restore must apply `normalizeResumedStatus` to the hub and every child (BUG-251), must not flip a `FAILED` cohort member to `done` (BUG-254/BUG-260), must keep per-node identity when nodes share an agent (BUG-256), and must let the placeholder-session hub reopen read-only (BUG-250).
- `T-5` The `TestBuildChatSessionSyncManifestRejectsNonChatRun` test is **replaced** (not deleted-to-pass) by a flow-run round-trip test; this CP-36 delta authorizes the behavior change (oracle-rule: spec conflict resolved upstream, not a test edited to make code pass).

### Constraints

- **MUST NOT regress** the Scenario 5/9/11 fixes: BUG-119/BUG-123 (child transcripts round-trip + no flatten), BUG-250 (placeholder-session reopen), BUG-251 (no stale `running` after rebuild), BUG-254 (failed reviewer keeps FAILED + identity), BUG-256 (shared-agent nodes keep distinct status), BUG-260 (resume fast-path never overwrites FAILED→done), BUG-263 (Chat picker `subMode`/`flowRef` survives reopen).
- MUST NOT regress normal chat sync/restore, cross-account resume (SD-14), or the existing single coder↔reviewer loop.
- MUST NOT change the flow engine coordination logic, node/edge/policy semantics, or agent interaction (CP-36 `P-7` additive-only).
- Flow/Step **definitions** stay on Supabase; only **run** transcript + runtime state syncs via Drive (CP-36 `P-5`). No new DB migration.
- Run GitNexus impact analysis before editing `BuildChatSessionSyncManifest`, `syncChatRunToDrive`, `restoreChatRunTreeFromDrive`, `isUnsyncedChat`, and `skipsResumeSessionValidation`; warn on HIGH/CRITICAL; `gitnexus_detect_changes()` clean before commit.

### Open Questions

- `Q-1` Does restoring `LoopState` + `ActiveFlowNodes/Edges` let Machine B *continue* a blocked/in-flight loop, or only *render* it? The guarantee here is **parity with a local post-restart reopen** (BUG-250/251: read-only reopen, children normalized to `cancelled`). Live continuation on a second machine is a follow-up if it needs orchestrator in-memory state not on disk.
- `Q-2` Should the hub's placeholder provider file be represented in the manifest as an explicit "no-transcript" marker, or should the manifest omit `ProviderFile` entirely for such runs? Decide during implementation; both restore-integrity and the desktop transcript viewer must tolerate it.

### Source Refs

- `CP-36` `P-5`, DOD-4/DOD-5, Scenario 5/6/9/11; `SD-19`; `Task-085`, `Task-103`.
- Code: `chat_session_sync.go` (`BuildChatSessionSyncManifest` ~L254, `syncChatRunToDrive`, `restoreChatRunTreeFromDrive` ~L667/L714-721/L805-816, `ChatSessionSyncManifest` ~L27-53); `interactive_handlers.go` `skipsResumeSessionValidation`; `interactive_resume.go` `normalizeResumedStatus`; `local_file_session_store.go` `ndjsonSessionRecord` L51-84; `chat_session_sync_test.go` `TestBuildChatSessionSyncManifestRejectsNonChatRun` L393, `TestBuildChatSessionSyncManifestMissingProviderFile` L406; desktop `navigatorHistory.ts` `isUnsyncedChat`, `Navigator.tsx`, `state/store.ts` `syncAllInProject`.

## 1. Goal

A FlowPilot user can select a flow-engine / workflow run in the desktop History panel, click **Sync**, and have it upload to Google Drive; on another machine (or after a local wipe) that run appears under **REMOTE CHATS**, restores, and reopens with its board showing the correct round, children, and status — reaching **parity with a local post-restart reopen** of the same run, with none of the Scenario 5/9/11 bugfixes regressed.

## 2. Parent Links

- coding plan: [CP-36: Generic Agent-Flow Engine And Review Loop](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) (`P-5`, DOD-4/DOD-5, Scenario 6)
- tech design: [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md)
- system spec: [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md)
- specific upstream ids: CP-36 `P-5`, DOD-5, Scenario 6; BUG-119/123 (child sync); BUG-250/251/254/256/260/263 (flow resume/restore invariants)

## 3. Trigger

User reported that clicking **Sync** on `[flow-engine joined result note…]` runs did nothing. Investigation confirmed those are flow-engine runs (`runKind != "chat"`), so the frontend `isUnsyncedChat` returns `false` → the selection Sync click filters to an empty list → `requestConfirm` early-returns on `runIds.length === 0` → no dialog, no error (silent no-op); and the backend rejects non-chat runs at `chat_session_sync.go:254`.

Re-reading CP-36 during triage surfaced two further facts: (a) Scenario 6 (Drive Sync Cross-PC) is the **only** CP-36 E2E scenario still unpassed, and it cannot pass while the gate exists; (b) **DOD-5 is checked** yet claims flow-run cross-PC Drive sync that no shipped code path delivers (the per-run sync is gated to chat; Task-103 syncs only context-engine data). This task both implements the capability and reconciles the plan.

## 4. Exact Change

- `T-1` **Backend — lift the sync gate.** In `chat_session_sync.go`, `BuildChatSessionSyncManifest` (~L254): replace `if session.RunKind != "chat"` with `isSyncableRunKind(session.RunKind)` (accepts chat + flow/workflow top-level runs). Update the stale `"only chat runs can be resumed in this version"` message.
- `T-2` **Backend — lift the restore gate.** Same file, `restoreChatRunTreeFromDrive` integrity check (~L717): replace `manifest.RunKind != "chat"` with the same rule.
- `T-3` **Backend — carry flow runtime state.** Extend `ChatSessionSyncManifest` (~L34-53) with additive optional fields mirroring `ndjsonSessionRecord`: `LoopState *AgentLoopState`, `AutoOrchestrate bool`, `FlowCohortID string`, `ActiveFlowEdges []agentpack.FlowEdge`, `ActiveFlowNodes []agentpack.FlowNode`, `PendingAgentContext []string`. Populate from the session in `BuildChatSessionSyncManifest`; restore into `ProviderSessionState` in `restoreChatRunTreeFromDrive` alongside the existing `RunKind`/`ParentRunID`/`DependsOn`/`AgentStatus` restore (~L805-816).
- `T-4` **Backend — tolerate the placeholder / missing provider file (BUG-250).** In `BuildChatSessionSyncManifest`, when the run's session id is still the synthetic placeholder (same invariant as `skipsResumeSessionValidation`), skip the provider-file resolution/upload instead of failing `session_unavailable`, and emit a manifest with an empty/omitted `ProviderFile`. In `restoreChatRunTreeFromDrive`, relax the integrity check (`ProviderFile.SHA256 != "" && RelativePath != ""` at ~L717-719) so a no-transcript flow hub manifest is valid; restore it as a read-only reopen (mirror BUG-250's degrade-to-read-only), still restoring flow state + children.
- `T-5` **Backend — apply hardened restore invariants.** On restore of the hub and each child, apply `normalizeResumedStatus` (BUG-251: no stale `running`), do not overwrite a `FAILED` cohort member to `done` (BUG-254/BUG-260), and preserve per-node identity/status when nodes share an agent (BUG-256). Ensure `ChildAgents` round-trips each child's real terminal `AgentStatus` (already uploaded per BUG-119) and restore honors it.
- `T-6` **Frontend — generalize the syncability predicate.** In `navigatorHistory.ts`, replace `isUnsyncedChat` with `isSyncableRun` (top-level chat OR flow/workflow, `!isAgentHistoryItem`, `syncStatus !== "synced"`, no `unavailableReason`). Update all `Navigator.tsx` references (`unsyncedCount`, per-row `showSync`, selection Sync filter).
- `T-7` **Frontend — align batch sync + kill the silent no-op.** In `state/store.ts` `syncAllInProject` (~L1370) switch the `runKind === "chat"` filter to the broadened rule. In `Navigator.tsx`, ensure the selection-mode **Sync** click always gives feedback (disable when nothing selected is syncable, or let the confirm dialog's existing "No syncable chats selected…" branch render) — never a silent dead click.
- `T-8` **Tests — replace the rejection test; add round-trips.** Replace `TestBuildChatSessionSyncManifestRejectsNonChatRun` (it pins the now-changed behavior; this CP-36 delta sanctions the change) with `TestBuildChatSessionSyncManifestAcceptsFlowRun` and a `syncChatRunToDrive` + `restoreChatRunTreeFromDrive` round-trip test that (a) syncs a flow hub run with placeholder session id + children, (b) restores on a fresh store, (c) asserts flow state fields survive, children keep their terminal `AgentStatus` (incl. a `FAILED` one), and no child shows `running`. Add `navigatorHistory` unit tests for `isSyncableRun`.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/chat_session_sync.go` (gates, manifest struct, build + restore, placeholder handling)
  - `apps/local-runner/internal/runner/chat_session_sync_test.go` (replace rejection test; add round-trip)
  - `apps/local-runner/internal/runner/interactive_handlers.go` / `interactive_resume.go` (reuse `skipsResumeSessionValidation`, `normalizeResumedStatus` — reuse, avoid duplicating)
  - `apps/desktop-flowpilot/src/components/navigatorHistory.ts` + `.test.ts`
  - `apps/desktop-flowpilot/src/components/Navigator.tsx` (references + no-op fix)
  - `apps/desktop-flowpilot/src/state/store.ts` (`syncAllInProject`)
- modules: local-runner chat-session-sync + resume; desktop navigator/history/store
- routes: existing `/client/workflow-runs/{runId}/sync-chat` + restore endpoints (no new routes)
- tables: none (Drive `chat-sessions/_index` + per-run manifest; no DB migration)

## 6. Acceptance Check

Feature:
- Selecting a flow-engine run and clicking **Sync** opens the confirm dialog and uploads to Drive; the row transitions `syncing → synced` (no silent no-op).
- `BuildChatSessionSyncManifest` returns a manifest (no `409 resume_unsupported`, no `session_unavailable`) for a flow/workflow run — including a hub run whose session id is the placeholder — and includes the flow runtime fields.
- CP-36 Scenario 6 (second machine): the flow run appears in **REMOTE CHATS**, restores, and its board renders the correct round/children/status; a `blocked` run shows the Extend/Stop prompt. This is what flips Scenario 6's three checkboxes and lets DOD-5 be truthfully checked.

Regression (must all still hold after the change):
- Normal chat sync/restore unchanged; BUG-119 child transcripts still upload and BUG-123 restore still rebuilds the tree without flattening.
- BUG-250: a restored placeholder-session flow hub reopens read-only, no `session_unavailable`.
- BUG-251: restored children show `cancelled`/terminal status, never stale `running`.
- BUG-254/BUG-256/BUG-260: a `FAILED` reviewer round-trips as `FAILED` with its own node identity; not flipped to `done`; distinct even when nodes share `agents/reviewer.md`.
- BUG-263: reopening a restored Chat-mode flow run keeps the Bug/Review Loop intent (verify `subMode`/`flowRef` survive if the desktop derives them from the restored run).
- `go build`/`go vet` clean; chat-sync runner tests + `navigatorHistory` tests pass; typecheck clean. Run the CP-36 §7 flow-persistence tests to confirm no resume regression.

## 7. Out of Scope

- Live re-invocation/continuation of a flow loop from a Drive-restored run beyond render/state parity with a local post-restart reopen (Q-1). If continuation needs orchestrator in-memory state not persisted to `sessions.ndjson`, it is a follow-up task.
- Any change to the flow engine coordination logic, node/edge/policy semantics, or agent interaction (CP-36 `P-7`).
- Moving flow/step **definitions** off Supabase; context-engine sync (Task-103) is untouched.
- Recursive grandchild sync depth changes (BUG-119 keeps one level).

## 8. Completion Notes

- result: `T-1`…`T-8` implemented and verified by automated checks; **live 2-machine Drive verification was NOT run** (no Google Drive credentials / second machine in this environment — same caveat BUG-119 recorded for its own live-sync check).
  - `T-1`/`T-2` — `isSyncableRunKind` replaces the `runKind != "chat"` gate in `BuildChatSessionSyncManifest` and the `restoreChatRunTreeFromDrive` integrity check.
  - `T-3` — `ChatSessionSyncManifest` carries `LoopState`/`AutoOrchestrate`/`FlowCohortID`/`ActiveFlowEdges`/`ActiveFlowNodes`/`PendingAgentContext`/`ChatSubMode`/`ChatFlowRef`; populated in `BuildChatSessionSyncManifest`, restored into `ProviderSessionState` in `restoreChatRunTreeFromDrive`.
  - `T-4` — new `resolveChatSessionTranscript` helper tolerates a placeholder (`"thread-*"`) or missing session for a flow/workflow run (empty `ProviderFile`, nil body); a `"chat"` run in the same state still hard-fails `session_unavailable` (unchanged, regression-tested). `uploadChatSessionRunFiles` and the restore integrity check both skip the transcript when `ProviderFile` is empty.
  - `T-5` — restore applies `normalizeResumedStatus` to the hub's `Status`/`AgentStatus` and to each `remapped` child summary before `setHistoricalChildren`; `normalizeResumedStatus` never touches `failed`/`completed`, so BUG-254/256/260's "don't overwrite FAILED" invariant is preserved by construction, not by new bespoke logic.
  - `T-6`/`T-7` — `isUnsyncedChat` moved out of `Navigator.tsx` into `navigatorHistory.ts` as exported `isSyncableRun`; `store.ts`'s `syncAllInProject` now uses it too. `requestConfirm`'s early-return no longer swallows a "sync" click that resolves to zero syncable items — the existing "No syncable chats selected…" dialog branch now actually renders instead of being dead code.
  - `T-8` — `TestBuildChatSessionSyncManifestRejectsNonChatRun` replaced by `TestBuildChatSessionSyncManifestAcceptsFlowHubWithPlaceholderSession` + `TestBuildChatSessionSyncManifestChatRunStillRejectsPlaceholderSession`; added `TestSyncAndRestoreFlowRunRoundTripAppliesFlowStateAndNormalizesStatus` (full sync→restore round trip on a fresh Machine-B-style store, asserting flow state round-trips and both the hub's and a `running` child's status normalize to `cancelled` on restore). Added 4 `isSyncableRun` cases to `navigatorHistory.test.ts`.
- verification actually run this turn:
  - `go build ./...` and `go vet ./...` clean (one pre-existing, unrelated `gitnexus.go` vet note untouched by this change).
  - `go test ./internal/runner/...` (targeted `ChatSession|Restore|Sync|Flow` and full package): only the same 2 pre-existing, environment-dependent failures as the unmodified baseline (confirmed via `git stash` diff-against-baseline) — a stale machine-specific `ProviderAccountID` fixture value and a missing local `codex` CLI binary. No new failures.
  - `npx tsc --noEmit` (desktop-flowpilot): clean.
  - `node --test` against the compiled `navigatorHistory.test.js`: 9/9 pass (5 pre-existing + 4 new `isSyncableRun` cases).
  - `store.test.ts` could not be executed standalone via plain `node --test` — its compiled output's `@/`-aliased value imports (e.g. `require("@/client/createRunnerClient")`) don't resolve outside whatever loader the project's real test harness uses; confirmed this is pre-existing and unrelated to this change (an already-existing `@/` value import in `store.js` fails identically with my edit reverted). `tsc --noEmit` already confirms the `store.ts` edit is type-correct.
  - GitNexus MCP tools (`gitnexus_impact` etc.) were not available in this session (only the CLI form is documented) — proceeded via manual grep/read verification of every call site instead, per repo policy for when GitNexus is unavailable. The Bash-hook's passive GitNexus lookups (triggered automatically on grep/sed calls) confirmed `isUnsyncedChat`'s only caller was `Navigator` before the rename.
  - **Not run:** a live 2-machine Google Drive sync + restore (CP-36 Scenario 6's actual manual E2E) — no Drive credentials / second machine available here.
- follow-ups: run CP-36 Scenario 6 live (2 machines, real Drive) and flip its 3 checkboxes; once confirmed, re-check DOD-5 with real cross-PC evidence instead of the "open" correction note; resolve Q-1 (does a Drive-restored flow actually continue/re-invoke on Machine B, or is it render-only like a local post-restart reopen — needs a live pass to answer).
- upstream docs updated:
  - CP-36 — delta note under **Scenario 6** (BLOCKED → this task) and under **DOD-5** (flow-run cross-PC Drive sync over-claimed; now implemented here, still awaiting live verification). Task-190 linked in Related Documents. To be updated again once Scenario 6's live pass runs.
