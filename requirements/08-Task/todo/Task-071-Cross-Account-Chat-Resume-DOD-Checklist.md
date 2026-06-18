# Task-071: Cross-Account Chat Resume Definition of Done Checklist

## Metadata

- Document ID: `Task-071`
- Title: `Cross-Account Chat Resume Definition of Done Checklist`
- Phase: `task`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-17`
- Last Updated: `2026-06-17`
- Parent Documents: [09-IG: Cross-Account Chat Resume Implementation Guide](../../10-Refactor/New-System/09-Cross-Account-Chat-Resume-Implementation-Guide.md), [Task-067: Desktop Post-Restart Run Resume Via Provider Session ID](./Task-067-Desktop-Post-Restart-Run-Resume-Via-Provider-Session-Id.md), [Task-068: Desktop History Unified View; Account ID As Local-File Pointer](./Task-068-Desktop-History-Unified-View-Account-As-Local-File-Pointer.md), [Task-069: Cross-PC Sync for Non-Supabase Users](./Task-069-Cross-PC-Sync-Non-Supabase-Sessions-Ndjson.md)
- Child Documents: `none`
- Related Documents: [BUG-080: Desktop Run History Lost On App Restart No Supabase](../../09-BugFix/done/BUG-080-Desktop-Run-History-Lost-On-App-Restart-No-Supabase.md), [BUG-060: Desktop Run History Empties After Switching Runs](../../09-BugFix/done/BUG-060-Desktop-Run-History-Empties-After-Switching-Runs.md), [Task-059: Desktop Check Version Tested Baseline Config](../done/Task-059-Desktop-Check-Version-Tested-Baseline-Config.md), [Task-057: Cross-PC Provider Chat Sync](./Task-057-\(Move-to-069\)-Cross-PC-Provider-Chat-Sync.md)
- Replaces: `none`
- Tags: `desktop, history, local-runner, resume, account-sync, codex, claude, dod`

## AI Quick View

### Summary

- This file is the implementation Definition of Done checklist for the cross-account chat resume feature.
- The source of truth for implementation sequencing remains [09-IG](../../10-Refactor/New-System/09-Cross-Account-Chat-Resume-Implementation-Guide.md); this document tracks what must be true before the feature can be called complete.
- Scope covers chat runs only: real provider session persistence, post-restart resume, same-PC cross-account continuation, desktop greyout fallback, tests, and compatibility canaries.
- Cross-PC Google Drive sync is represented only as readiness/foundation checks; full cross-PC sync remains Task-069.

### Current Ask

- Use this checklist while implementing the feature and during final review to verify every required behavior, regression guard, and scope boundary.

### Key Decisions

- `T-1` `run_id` remains the durable user-facing identity; `provider_session_id` and `provider_account_id` are mutable internal pointers.
- `T-2` History remains one unified list with no account grouping, badges, or filters.
- `T-3` Unopenable history items stay visible but are greyed-out / disabled with a clear reason.
- `T-4` MVP completion means chat runs only. Workflow-run resume and full cross-PC sync are not required for this DoD.
- `T-5` Codex reopened runs resume through `codex exec resume <session_id>`, not the app-server fresh-thread path.

### Constraints

- Do not store or log provider session file contents.
- Keep `sessions.ndjson` backward-compatible; old entries without `provider_account_id` must load.
- Do not modify `fakeWorkflowStore` for disk persistence behavior.
- Do not add account UI to the desktop history list.
- Do not implement automatic chat sync; any future cross-PC sync is explicit user action only.

### Open Questions

- `Q-1` Claude cross-account portability still needs a real-account manual verification if not already available in the implementation environment.
- `Q-2` Final Codex `codex exec resume` output parsing should be confirmed against the tested Codex version during implementation.
- `Q-3` Cross-PC `run_id` collision strategy remains Task-069 scope and does not block this same-PC DoD.

### Source Refs

- [09-IG: Cross-Account Chat Resume Implementation Guide](../../10-Refactor/New-System/09-Cross-Account-Chat-Resume-Implementation-Guide.md)
- [Task-067: Desktop Post-Restart Run Resume Via Provider Session ID](./Task-067-Desktop-Post-Restart-Run-Resume-Via-Provider-Session-Id.md)
- [Task-068: Desktop History Unified View; Account ID As Local-File Pointer](./Task-068-Desktop-History-Unified-View-Account-As-Local-File-Pointer.md)
- [Task-069: Cross-PC Sync for Non-Supabase Users](./Task-069-Cross-PC-Sync-Non-Supabase-Sessions-Ndjson.md)
- [BUG-080: Desktop Run History Lost On App Restart No Supabase](../../09-BugFix/done/BUG-080-Desktop-Run-History-Lost-On-App-Restart-No-Supabase.md)
- [Task-059: Desktop Check Version Tested Baseline Config](../done/Task-059-Desktop-Check-Version-Tested-Baseline-Config.md)

## 1. Goal

Provide one reviewable checklist for declaring the cross-account chat resume feature done. The feature is complete only when a chat run can be reopened and continued after runner restart, can be continued under another signed-in provider account on the same PC when the provider session file is portable, and fails visibly and safely when it cannot be opened.

## 2. Parent Links

- coding plan: [09-IG: Cross-Account Chat Resume Implementation Guide](../../10-Refactor/New-System/09-Cross-Account-Chat-Resume-Implementation-Guide.md)
- tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: Task-067, Task-068, Task-069, BUG-080, BUG-060, Task-059

## 3. Trigger

The project now has enough design detail to implement the cross-account chat resume slice. A separate DoD checklist is needed so implementation agents can verify the whole feature without missing foundation items from Task-067, account-pointer items from Task-068, compatibility canaries from Task-059, or cross-PC readiness constraints from Task-069.

## 4. Exact Change

### Persistence And Resume Handle

- [x] `DOD-01` Add `provider_account_id` to `ndjsonSessionRecord` with `omitempty`.
- [x] `DOD-02` Map `ProviderSessionState.ProviderAccountID` through `sessionRecordFrom`.
- [x] `DOD-03` Map `provider_account_id` through `sessionStateFromRecord`.
- [x] `DOD-04` Existing `sessions.ndjson` lines without `provider_account_id` still load without error.
- [x] `DOD-05` `UpsertProviderSession` remains last-wins by `run_id` for both in-memory state and NDJSON reload.
- [x] `DOD-06` `sessionStateOf` persists the real provider resume handle when known, falling back to the synthetic FlowPilot session id only when no real handle is available.
- [x] `DOD-07` `interactiveRun` stores the real provider session id separately from the synthetic FlowPilot id where needed.
- [x] `DOD-08` Claude real session id captured from provider output is persisted back to the session store.
- [x] `DOD-09` Codex rollout session id is persisted when exposed; if not exposed, a documented rollout discovery fallback is implemented.
- [x] `DOD-10` Persisted provider session rows include `RunKind`, `LastPrompt`, `LastMessage`, `StartedAt`, and `UpdatedAt` exactly as BUG-080 requires.

### Post-Restart Run Reconstruction

- [x] `DOD-11` `SessionHistoryReader` exposes `GetProviderSession(ctx, runID) (ProviderSessionState, bool, error)`.
- [x] `DOD-12` `localFileSessionStore.GetProviderSession` reads from the loaded last-wins in-memory session map.
- [x] `DOD-13` `SupabaseWorkflowStore` either implements the new lookup or the interface split keeps Supabase compilation and history behavior intact.
- [x] `DOD-14` `resumeRun` keeps the existing in-memory fast path unchanged when `s.runs[runID]` exists.
- [x] `DOD-15` `resumeRun` falls back to persisted session lookup when `s.runs[runID]` is missing.
- [x] `DOD-16` Missing persisted session still returns `run_not_found`.
- [x] `DOD-17` Non-chat restored runs are blocked with a typed unsupported/greyout-safe response for MVP.
- [x] `DOD-18` Chat restored runs are reconstructed into `interactiveRun` with provider key, account id, cwd, status, prompt/message, timestamps, and real provider session id.
- [x] `DOD-19` Reconstructed chat runs are registered back into `s.runs`.
- [x] `DOD-20` Reconstructed chat runs reseed the deterministic `chat-<runId>` step.
- [x] `DOD-21` Returned `RunHandle` includes the correct `StepID` for chat runs after reconstruction.

### Provider Account Home And Session Files

- [x] `DOD-22` Account home resolution works for explicit provider accounts.
- [x] `DOD-23` Account home resolution falls back correctly for empty/default account ids.
- [x] `DOD-24` Missing account home produces a typed, user-safe error.
- [x] `DOD-25` Provider auth is checked in the active account home before attempting cross-account resume.
- [x] `DOD-26` Missing active-account auth produces a typed greyout-safe error.
- [x] `DOD-27` Codex session locator finds rollout files under `<CODEX_HOME>/sessions/**/rollout-*-<sessionID>.jsonl`.
- [x] `DOD-28` Claude session locator finds session files under the provider account's Claude projects directory.
- [x] `DOD-29` Claude relocation preserves or derives the correct project hash directory from the source path.
- [x] `DOD-30` Relocation copies provider session files additively and never overwrites unrelated files.
- [x] `DOD-31` Session file contents are never logged.
- [x] `DOD-32` Missing provider session file returns `session_unavailable` or equivalent typed error with message `session data not found on this machine`.

### Cross-Account Resume

- [x] `DOD-33` If persisted `provider_account_id` matches `activeAccountID`, resume does not relocate files.
- [x] `DOD-34` If persisted `provider_account_id` differs from `activeAccountID`, the runner attempts file relocation into the active account home.
- [x] `DOD-35` Successful cross-account preparation re-points the same `run_id` to the active `provider_account_id`.
- [x] `DOD-36` Re-pointing persists via `UpsertProviderSession`.
- [x] `DOD-37` Re-pointing does not create a second logical history run.
- [x] `DOD-38` If provider resume rejects the relocated file, the run remains visible in history and becomes disabled/greyed-out with a clear reason.
- [x] `DOD-39` No transcript-history injection fallback is implemented.

### Claude Resume Path

- [x] `DOD-40` Restored Claude runs seed the process pool with the real session id before the next turn.
- [x] `DOD-41` Claude next turn uses `--resume <realSessionId>` and never resumes with the synthetic FlowPilot id.
- [x] `DOD-42` Claude turn environment points to the active account home after cross-account relocation.
- [ ] `DOD-43` Claude same-account post-restart resume sends a follow-up message successfully in manual or integration validation.
- [ ] `DOD-44` Claude cross-account behavior is either manually verified or explicitly documented as provider-untested with greyout fallback preserved.

### Codex Resume Path

- [x] `DOD-45` Fresh Codex runs continue to use the existing app-server path.
- [x] `DOD-46` Reopened/restored Codex chat runs use a CLI-backed resume path.
- [x] `DOD-47` Codex resume command uses `codex exec resume <rolloutSessionID>` with `CODEX_HOME` set to the active account home.
- [x] `DOD-48` Codex resume command runs from the restored run working directory.
- [x] `DOD-49` Codex sandbox and approval posture are derived from the existing yolo policy helper.
- [x] `DOD-50` Codex resume output is mapped into provider events that the desktop can render.
- [x] `DOD-51` If structured Codex JSON streaming is unavailable, final stdout is emitted as one completed assistant message plus turn-completed event.
- [ ] `DOD-52` Codex same-account post-restart resume sends a follow-up message successfully in manual or integration validation.
- [ ] `DOD-53` Codex cross-account resume under another signed-in account is manually validated using copied rollout file behavior from CA-098.

### Desktop History Behavior

- [x] `DOD-54` History list remains unified across provider accounts.
- [x] `DOD-55` No account badges, account grouping, or account filters are added.
- [x] `DOD-56` Clicking a resumable post-restart history item opens it instead of returning `run_not_found`.
- [x] `DOD-57` Clicking a resumable cross-account history item opens the same `run_id`.
- [x] `DOD-58` On typed resume errors, the history item is shown greyed-out / disabled.
- [x] `DOD-59` Greyed-out items show the server-provided reason in tooltip or equivalent UI affordance.
- [x] `DOD-60` Greyed-out click handling does not navigate, clear current timeline, or crash.
- [x] `DOD-61` Successful resume still clears/rebuilds the timeline through the existing stream replay path.
- [x] `DOD-62` Existing in-memory resume behavior is unchanged.

### Compatibility Canaries

- [x] `DOD-63` `scripts/quicktest.ps1` includes a session portability section.
- [x] `DOD-64` Quicktest checks that the Codex `exec resume` CLI surface still exists.
- [x] `DOD-65` Quicktest checks enough Codex rollout metadata shape to catch account/auth fields being introduced.
- [x] `DOD-66` Quicktest includes a Claude session-store/path canary where feasible without spending tokens.
- [x] `DOD-67` The Check Version flow and `quicktest.ps1` continue to use `.flowpilot/settings/compat-config.json` baseline values.

### Cross-PC Readiness Only

- [x] `DOD-68` The implementation keeps `run_id` stable and provider/account pointers mutable so Task-069 can re-point restored runs later.
- [x] `DOD-69` The implementation does not hard-code assumptions that prevent later workflow-run reconstruction by `workflow_id`.
- [x] `DOD-70` The implementation does not add automatic Google Drive chat sync.
- [x] `DOD-71` The implementation does not attempt to solve PC-to-PC `run_id` collisions in this slice.

### Tests And Regression Guards

- [x] `DOD-72` Local file session store tests cover `provider_account_id` round-trip.
- [x] `DOD-73` Local file session store tests cover last-wins re-pointing of `provider_session_id`.
- [x] `DOD-74` Local file session store tests cover old NDJSON records without `provider_account_id`.
- [x] `DOD-75` Unit tests cover `GetProviderSession` found and not-found behavior.
- [x] `DOD-76` Unit tests cover `resumeRun` fallback reconstruction from persisted session.
- [x] `DOD-77` Unit tests cover unsupported non-chat restore behavior.
- [x] `DOD-78` Unit tests cover cross-account missing source file error mapping.
- [x] `DOD-79` Unit tests cover cross-account missing active-account auth error mapping.
- [x] `DOD-80` Unit tests cover successful re-point persistence after relocation.
- [x] `DOD-81` Unit or integration tests cover Claude real-session persistence.
- [x] `DOD-82` Unit or integration tests cover Codex resume command construction.
- [x] `DOD-83` Desktop tests or focused manual QA cover greyed-out history item behavior.
- [x] `DOD-84` Existing BUG-060 and BUG-080 history tests still pass.
- [x] `DOD-85` `go test ./internal/runner/...` passes in `apps/local-runner`.
- [x] `DOD-86` Desktop TypeScript typecheck/build passes for touched UI/client files.
- [x] `DOD-87` No new test relies on real provider tokens unless marked manual.

### Manual End-To-End Checks

- [ ] `DOD-88` Start a chat, complete a turn, restart runner/app, click history item, and send a follow-up successfully.
- [ ] `DOD-89` Delete or move the provider session file, restart runner/app, click history item, and verify greyout/error reason instead of crash.
- [ ] `DOD-90` With two signed-in accounts on one PC, start chat under account A, switch to account B, click the account A history item, and continue under account B when provider portability allows it.
- [ ] `DOD-91` With active account not signed in, click a history item that requires that account and verify the signed-out reason is shown.
- [ ] `DOD-92` Verify the history list order and display fields still match BUG-080 behavior after restart.
- [ ] `DOD-93` Verify no provider session file contents appear in logs during the manual flow.

### Final Review Gate

- [x] `DOD-94` All HIGH/CRITICAL GitNexus impact warnings, if any appeared during implementation, were reviewed before edits proceeded.
- [ ] `DOD-95` `gitnexus_detect_changes()` was run before commit or final handoff.
- [x] `DOD-96` A reviewer confirms the implementation follows [09-IG](../../10-Refactor/New-System/09-Cross-Account-Chat-Resume-Implementation-Guide.md) rather than redesigning the feature.
- [x] `DOD-97` Any unimplemented checklist item is moved to a named follow-up task with an explicit reason.
- [x] `DOD-98` Completion notes in Task-067, Task-068, Task-069, and this file are updated if implementation status changes their meaning.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/local_file_session_store.go`
  - `apps/local-runner/internal/runner/workflow_store.go`
  - `apps/local-runner/internal/runner/interactive_handlers.go`
  - `apps/local-runner/internal/runner/interactive_service.go`
  - `apps/local-runner/internal/runner/claude_adapter.go`
  - `apps/local-runner/internal/runner/codex_adapter.go`
  - `apps/local-runner/internal/runner/codex_resume_process.go`
  - `apps/local-runner/internal/runner/session_file_locator.go`
  - `apps/desktop-flowpilot/src/components/Navigator.tsx`
  - `apps/desktop-flowpilot/src/state/store.ts`
  - `scripts/quicktest.ps1`
- modules:
  - `local-runner`
  - `desktop-flowpilot`
  - compatibility quicktest scripts
- routes:
  - `POST /client/workflow-runs/{runId}/resume`
  - `GET /client/projects/{projectId}/workflow-runs`
- tables:
  - `workflow_provider_sessions` for Supabase parity only if the interface change requires it

## 6. Acceptance Check

- Every checklist item from `DOD-01` through `DOD-98` is either checked or explicitly moved to a follow-up with a reason.
- Post-restart same-account chat resume works for at least one provider.
- Cross-account same-PC chat resume works for every provider marked supported, and unsupported provider/account cases grey out safely.
- History remains unified and visible even when an item cannot be opened.
- Regression tests for local history persistence and desktop history loading still pass.

## 7. Out of Scope

- Full cross-PC Google Drive chat sync implementation.
- Workflow-run reconstruction/resume.
- Account badges, grouping, or filtering in history.
- Automatic chat sync.
- Transcript replay as a fallback.
- Supabase-only chat sync implementation from Task-057.

## 8. Completion Notes

- result: Runner persistence, post-restart reconstruction, cross-account session relocation, Codex CLI resume, desktop greyout fallback, quicktest portability canaries, and focused local-runner tests were implemented.
- verification:
  - `go test ./internal/runner/...` passes in `apps/local-runner` after replacing platform-specific fake process fixtures and proxy-MCP auth setup with local mocks.
  - Focused BUG-060/BUG-080 history tests pass as part of the runner package.
- follow-ups:
  - Manual/E2E acceptance items remain open for same-account and cross-account live-provider validation (`DOD-43`, `DOD-44`, `DOD-52`, `DOD-53`, `DOD-88` to `DOD-93`).
  - `DOD-95` remains open because `gitnexus_detect_changes()` could not be run: GitNexus MCP impact/detect tools were not exposed in this session.
  - `DOD-96` closed: an independent reviewer confirmed the implementation follows 09-IG (all 7 load-bearing decisions — disk reconstruction, run_id-durable/pointer-mutable re-point, cross-account file relocation, typed greyout fallback with no history injection, Codex CLI `exec resume` path, Claude `--resume <realSessionId>`, generic reconstruction primitive) with no deviations.
  - `gitnexus_detect_changes()` could not be run because the GitNexus MCP impact/detect tools were not exposed in this session; CLI index refresh and direct scope checks were used instead.
- upstream docs updated: `Task-071` and `Task-072` completion state updated in-place.
