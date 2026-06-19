# Task-072: Cross-Account Chat Resume Test Signatures

## Metadata

- Document ID: `Task-072`
- Title: `Cross-Account Chat Resume Test Signatures`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-17`
- Last Updated: `2026-06-17`
- Parent Documents: [Task-071: Cross-Account Chat Resume Definition of Done Checklist](./Task-071-Cross-Account-Chat-Resume-DOD-Checklist.md), [09-IG: Cross-Account Chat Resume Implementation Guide](../../10-Refactor/New-System/09-Cross-Account-Chat-Resume-Implementation-Guide.md)
- Child Documents: `none`
- Related Documents: [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md), [Task-067: Desktop Post-Restart Run Resume Via Provider Session ID](./Task-067-Desktop-Post-Restart-Run-Resume-Via-Provider-Session-Id.md), [Task-068: Desktop History Unified View; Account ID As Local-File Pointer](./Task-068-Desktop-History-Unified-View-Account-As-Local-File-Pointer.md), [Task-069: Cross-PC Sync for Non-Supabase Users](./Task-069-Cross-PC-Sync-Non-Supabase-Sessions-Ndjson.md), [BUG-080: Desktop Run History Lost On App Restart No Supabase](../../09-BugFix/done/BUG-080-Desktop-Run-History-Lost-On-App-Restart-No-Supabase.md)
- Replaces: `none`
- Tags: `desktop, local-runner, tests, tdd, history, resume, codex, claude`

## AI Quick View

### Summary

- This file defines the full test-signature set for the cross-account chat resume implementation.
- Each test lists the target file, signature/name, expected input, expected output, and covered DoD IDs.
- The tests are intentionally implementation-facing: they should be written before or alongside the code changes from [09-IG](../../10-Refactor/New-System/09-Cross-Account-Chat-Resume-Implementation-Guide.md).
- Real provider-token tests are marked manual/e2e and must stay skipped by default.

### Current Ask

- Implement these tests as the acceptance harness for Task-071 before calling the feature complete.

### Key Decisions

- `T-1` Unit tests must use temp dirs, fake stores, fake provider homes, and fake command runners where possible.
- `T-2` Manual/e2e tests may require real Codex or Claude auth, but they must be opt-in through environment variables.
- `T-3` Desktop tests focus on state transitions and greyout behavior; visual QA remains a manual acceptance check unless the UI test framework is already active.
- `T-4` Cross-PC tests are limited to foundation behavior because Task-069 owns full Google Drive sync.

### Constraints

- Do not create tests that read or log real provider session contents.
- Do not require real provider tokens in normal `go test`.
- Do not make automatic Google Drive calls in this test set.
- Keep test data deterministic and local to `t.TempDir()`.

### Open Questions

- `Q-1` The final Codex CLI wrapper shape may change after implementation; keep command-construction assertions stable around argv/env/cwd, not private helper names.
- `Q-2` The desktop test runner location is not fixed in this doc; use the existing renderer/client-core test pattern when implementing.

### Source Refs

- [Task-071: Cross-Account Chat Resume Definition of Done Checklist](./Task-071-Cross-Account-Chat-Resume-DOD-Checklist.md)
- [09-IG: Cross-Account Chat Resume Implementation Guide](../../10-Refactor/New-System/09-Cross-Account-Chat-Resume-Implementation-Guide.md)
- [BUG-080: Desktop Run History Lost On App Restart No Supabase](../../09-BugFix/done/BUG-080-Desktop-Run-History-Lost-On-App-Restart-No-Supabase.md)
- `apps/local-runner/internal/runner/local_file_session_store_test.go`
- `apps/local-runner/internal/runner/claude_adapter_test.go`
- `apps/local-runner/internal/runner/codex_appserver_test.go`
- `apps/local-runner/internal/runner/bug060_test.go`

## 1. Goal

Define the exact test signatures, inputs, and expected outputs needed to verify the full cross-account chat resume feature. This document is not implementation code; it is the TDD checklist that implementation agents should convert into Go, TypeScript, script, and manual/e2e tests.

## 2. Parent Links

- coding plan: [09-IG: Cross-Account Chat Resume Implementation Guide](../../10-Refactor/New-System/09-Cross-Account-Chat-Resume-Implementation-Guide.md)
- tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: Task-071 `DOD-01` through `DOD-98`

## 3. Trigger

Task-071 defines what done means. This task turns that checklist into concrete test names and expected input/output contracts so the implementation can be built against an explicit harness instead of relying on ad hoc validation.

## 4. Exact Change

### Local File Session Store Tests

#### `[done] TS-001` Provider account id round-trip

- target file: `apps/local-runner/internal/runner/local_file_session_store_test.go`
- signature: `func TestLocalFileSessionStoreProviderAccountIDRoundTrip(t *testing.T)`
- expected input:
  - temp store directory
  - `ProviderSessionState{RunID:"run-1", ProjectID:"project-1", ProviderKey:ProviderCodex, ProviderSessionID:"rollout-1", ProviderAccountID:"acct-a", WorkingDirectory:"/repo", Status:RunStatusCompleted, RunKind:"chat"}`
  - call `UpsertProviderSession`, recreate store with same directory, call `ListProviderSessionsByProject`
- expected output:
  - exactly one session
  - `ProviderAccountID == "acct-a"`
  - `ProviderSessionID == "rollout-1"`
  - no load error
- covers: `DOD-01`, `DOD-02`, `DOD-03`, `DOD-72`

#### `[done] TS-002` Old NDJSON without account id remains valid

- target file: `apps/local-runner/internal/runner/local_file_session_store_test.go`
- signature: `func TestLocalFileSessionStoreLoadsLegacyRecordWithoutProviderAccountID(t *testing.T)`
- expected input:
  - temp `sessions.ndjson` containing one JSON line without `provider_account_id`
  - call `NewLocalFileSessionStore`, then `GetProviderSession(ctx, "run-legacy")`
- expected output:
  - `found == true`
  - `ProviderAccountID == ""`
  - legacy display fields still map if present
  - no panic and no decode error
- covers: `DOD-04`, `DOD-74`

#### `[done] TS-003` Last-wins re-points provider session and account

- target file: `apps/local-runner/internal/runner/local_file_session_store_test.go`
- signature: `func TestLocalFileSessionStoreLastWinsRepointsProviderSessionAndAccount(t *testing.T)`
- expected input:
  - first upsert: `RunID:"run-1", ProviderSessionID:"old-session", ProviderAccountID:"acct-a"`
  - second upsert: same `RunID`, `ProviderSessionID:"new-session", ProviderAccountID:"acct-b"`
  - recreate store and fetch `run-1`
- expected output:
  - one logical session for `run-1`
  - `ProviderSessionID == "new-session"`
  - `ProviderAccountID == "acct-b"`
  - project history list contains one item, not two
- covers: `DOD-05`, `DOD-35`, `DOD-36`, `DOD-37`, `DOD-73`

#### `[done] TS-004` GetProviderSession found and not found

- target file: `apps/local-runner/internal/runner/local_file_session_store_test.go`
- signature: `func TestLocalFileSessionStoreGetProviderSession(t *testing.T)`
- expected input:
  - temp store with `run-1`
  - call `GetProviderSession(ctx, "run-1")`
  - call `GetProviderSession(ctx, "missing-run")`
- expected output:
  - for `run-1`: `(state, true, nil)` and state fields match input
  - for `missing-run`: `(zero state, false, nil)`
- covers: `DOD-11`, `DOD-12`, `DOD-75`

#### `[done] TS-005` Supabase lookup contract stays satisfied

- target file: `apps/local-runner/internal/runner/supabase_workflow_store_test.go`
- signature: `func TestSupabaseWorkflowStoreGetProviderSession(t *testing.T)`
- expected input:
  - fake PostgREST handler responding to a single-row provider session query for `workflow_run_id=eq.run-1`
  - returned row includes provider key, provider session id, provider account id, working directory, status, and joined workflow run fields
- expected output:
  - `(state, true, nil)`
  - `RunID == "run-1"`
  - `ProviderSessionID == "sess-1"`
  - `ProviderAccountID == "acct-a"` if Supabase path includes it
  - not-found response returns `found == false`
- covers: `DOD-13`, `DOD-75`

### Session Snapshot And Real Handle Tests

#### `[done] TS-006` sessionStateOf prefers real provider session id

- target file: `apps/local-runner/internal/runner/interactive_service_test.go`
- signature: `func TestSessionStateOfUsesRealProviderSessionIDWhenKnown(t *testing.T)`
- expected input:
  - `interactiveRun{providerSessionID:"thread-1", realProviderSessionID:"claude-real-1", providerAccountID:"acct-a", runKind:"chat"}`
  - call `sessionStateOf`
- expected output:
  - `ProviderSessionID == "claude-real-1"`
  - `ProviderAccountID == "acct-a"`
  - `RunKind == "chat"`
- covers: `DOD-06`, `DOD-07`

#### `[done] TS-007` sessionStateOf falls back to synthetic id

- target file: `apps/local-runner/internal/runner/interactive_service_test.go`
- signature: `func TestSessionStateOfFallsBackToSyntheticProviderSessionID(t *testing.T)`
- expected input:
  - `interactiveRun{providerSessionID:"thread-1", realProviderSessionID:""}`
  - call `sessionStateOf`
- expected output:
  - `ProviderSessionID == "thread-1"`
- covers: `DOD-06`

#### `TS-008` Claude real session persists into service session state

- target file: `apps/local-runner/internal/runner/claude_adapter_test.go`
- signature: `func TestClaudeAdapterPersistsRealSessionIDForResume(t *testing.T)`
- expected input:
  - fake Claude stream emits `system/init` or `result` line with `session_id:"claude-real-1"`
  - `TurnRequest{RunID:"run-1", ProviderSessionID:"thread-1", Cwd:"/repo"}`
  - fake persist callback records `ProviderSessionState`
- expected output:
  - pool maps `"thread-1" -> "claude-real-1"`
  - persisted state has `ProviderSessionID == "claude-real-1"`
  - persisted state keeps `RunID == "run-1"` and `WorkingDirectory == "/repo"`
- covers: `DOD-08`, `DOD-81`

#### `[done] TS-009` Codex rollout discovery fallback chooses newest matching cwd

- target file: `apps/local-runner/internal/runner/session_file_locator_test.go`
- signature: `func TestDiscoverCodexRolloutSessionIDChoosesNewestMatchingCWD(t *testing.T)`
- expected input:
  - fake Codex home with multiple rollout JSONL files
  - two files contain session meta for `/repo`, one older and one newer
  - one newer unrelated file contains cwd `/other`
  - call discovery helper with `cwd="/repo"`
- expected output:
  - returns session id from newest `/repo` rollout
  - ignores unrelated cwd
  - does not read or return conversation body
- covers: `DOD-09`, `DOD-31`

### Post-Restart Resume Reconstruction Tests

#### `[done] TS-010` resumeRun reconstructs persisted chat run

- target file: `apps/local-runner/internal/runner/interactive_handlers_test.go`
- signature: `func TestResumeRunReconstructsChatRunFromDisk(t *testing.T)`
- expected input:
  - service with empty `s.runs`
  - store contains `ProviderSessionState{RunID:"run-1", ProjectID:"project-1", ProviderKey:ProviderClaude, ProviderSessionID:"claude-real-1", ProviderAccountID:"acct-a", WorkingDirectory:"/repo", Status:RunStatusCompleted, LastPrompt:"hello", LastMessage:"done", RunKind:"chat"}`
  - `activeAccountID == "acct-a"`
  - call `resumeRun("run-1")`
- expected output:
  - `apiErr == nil`
  - `RunHandle{RunID:"run-1", ProviderSessionID:"claude-real-1", ProviderKey:ProviderClaude, StepID:"chat-run-1"}`
  - `s.runs["run-1"]` exists
  - reconstructed run has prompt/message/cwd/account fields from store
- covers: `DOD-15`, `DOD-18`, `DOD-19`, `DOD-21`, `DOD-76`

#### `[done] TS-011` resumeRun reseeds chat step after reconstruction

- target file: `apps/local-runner/internal/runner/interactive_handlers_test.go`
- signature: `func TestResumeRunReconstructsChatRunSeedsChatStep(t *testing.T)`
- expected input:
  - same setup as `TS-010`
  - after `resumeRun`, call `workflowStore.LoadRunSteps(ctx, "run-1")`
- expected output:
  - one step exists
  - step id is `chat-run-1`
  - step type is `chat`
  - status is pending or expected initial resumable status
- covers: `DOD-20`, `DOD-76`

#### `[done] TS-012` resumeRun preserves in-memory fast path

- target file: `apps/local-runner/internal/runner/interactive_handlers_test.go`
- signature: `func TestResumeRunUsesInMemoryRunBeforeDiskLookup(t *testing.T)`
- expected input:
  - service has `s.runs["run-1"]` with provider session id `"in-memory-session"`
  - store has same run id with provider session id `"disk-session"`
  - call `resumeRun("run-1")`
- expected output:
  - handle uses `"in-memory-session"`
  - no reconstruction side effects
  - no disk value overrides current live run
- covers: `DOD-14`, `DOD-62`

#### `[done] TS-013` resumeRun missing persisted session returns run_not_found

- target file: `apps/local-runner/internal/runner/interactive_handlers_test.go`
- signature: `func TestResumeRunMissingPersistedSessionReturnsRunNotFound(t *testing.T)`
- expected input:
  - service with empty `s.runs`
  - empty session store
  - call `resumeRun("missing-run")`
- expected output:
  - API error status `404`
  - API error code `run_not_found`
  - no new run inserted into `s.runs`
- covers: `DOD-16`, `DOD-75`

#### `[done] TS-014` resumeRun rejects non-chat restored run

- target file: `apps/local-runner/internal/runner/interactive_handlers_test.go`
- signature: `func TestResumeRunRestoredWorkflowRunUnsupportedForMVP(t *testing.T)`
- expected input:
  - store contains `ProviderSessionState{RunID:"run-workflow", RunKind:"workflow"}`
  - `s.runs` empty
  - call `resumeRun("run-workflow")`
- expected output:
  - typed API error status `409`
  - code `resume_unsupported` or agreed greyout-safe equivalent
  - message says only chat runs can be resumed in this version
  - no run inserted
- covers: `DOD-17`, `DOD-77`

### Account Home And Session File Locator Tests

#### `[done] TS-015` resolveAccountHome explicit account

- target file: `apps/local-runner/internal/runner/provider_accounts_test.go`
- signature: `func TestResolveAccountHomeExplicitAccount(t *testing.T)`
- expected input:
  - provider accounts registry contains Codex account `acct-a` with home `/tmp/codex-a`
  - call `resolveAccountHome(ProviderCodex, "acct-a")`
- expected output:
  - returns `"/tmp/codex-a", true`
- covers: `DOD-22`

#### `[done] TS-016` resolveAccountHome default account fallback

- target file: `apps/local-runner/internal/runner/provider_accounts_test.go`
- signature: `func TestResolveAccountHomeDefaultAccountFallback(t *testing.T)`
- expected input:
  - default provider home detector returns `/tmp/codex-default`
  - call `resolveAccountHome(ProviderCodex, "")`
  - call `resolveAccountHome(ProviderCodex, "default")`
- expected output:
  - both calls return `/tmp/codex-default, true`
- covers: `DOD-23`

#### `[done] TS-017` resolveAccountHome missing account

- target file: `apps/local-runner/internal/runner/provider_accounts_test.go`
- signature: `func TestResolveAccountHomeMissingAccount(t *testing.T)`
- expected input:
  - registry has no `acct-missing`
  - call `resolveAccountHome(ProviderClaude, "acct-missing")`
- expected output:
  - returns `("", false)`
  - caller maps this to typed API error
- covers: `DOD-24`

#### `[done] TS-018` LocateSessionFile finds Codex rollout by id

- target file: `apps/local-runner/internal/runner/session_file_locator_test.go`
- signature: `func TestLocateSessionFileCodexFindsRolloutBySessionID(t *testing.T)`
- expected input:
  - temp Codex home with `sessions/2026/06/17/rollout-2026-06-17T01-02-03-rollout-abc.jsonl`
  - call `LocateSessionFile(ProviderCodex, home, "rollout-abc", "/repo")`
- expected output:
  - `found == true`
  - returned absolute path is the rollout file
- covers: `DOD-27`

#### `[done] TS-019` LocateSessionFile returns false for missing Codex rollout

- target file: `apps/local-runner/internal/runner/session_file_locator_test.go`
- signature: `func TestLocateSessionFileCodexMissing(t *testing.T)`
- expected input:
  - temp Codex home with no matching rollout suffix
  - call `LocateSessionFile(ProviderCodex, home, "missing-id", "/repo")`
- expected output:
  - path `""`
  - `found == false`
- covers: `DOD-32`

#### `[done] TS-020` LocateSessionFile finds Claude session

- target file: `apps/local-runner/internal/runner/session_file_locator_test.go`
- signature: `func TestLocateSessionFileClaudeFindsProjectSession(t *testing.T)`
- expected input:
  - temp account home with `.claude/projects/project-hash/claude-real-1.jsonl`
  - call `LocateSessionFile(ProviderClaude, home, "claude-real-1", "/repo")`
- expected output:
  - `found == true`
  - returned absolute path ends with `.claude/projects/project-hash/claude-real-1.jsonl`
- covers: `DOD-28`

#### `[done] TS-021` RelocateSessionFile copies Codex rollout additively

- target file: `apps/local-runner/internal/runner/session_file_locator_test.go`
- signature: `func TestRelocateSessionFileCodexCopiesRolloutWithoutOverwrite(t *testing.T)`
- expected input:
  - source rollout path under source home
  - target Codex home without the rollout
  - call `RelocateSessionFile(ProviderCodex, src, targetHome, "rollout-abc", "/repo")`
- expected output:
  - destination file exists under target `sessions/...`
  - file bytes equal source bytes
  - existing unrelated target files remain unchanged
- covers: `DOD-30`, `DOD-31`

#### `[done] TS-022` RelocateSessionFile refuses overwrite

- target file: `apps/local-runner/internal/runner/session_file_locator_test.go`
- signature: `func TestRelocateSessionFileDoesNotOverwriteExistingSessionFile(t *testing.T)`
- expected input:
  - source session file bytes `"source"`
  - target destination already exists with bytes `"target"`
  - call `RelocateSessionFile`
- expected output:
  - returns an error or no-op according to final helper contract
  - target bytes remain `"target"`
- covers: `DOD-30`

#### `[done] TS-023` RelocateSessionFile preserves Claude project hash directory

- target file: `apps/local-runner/internal/runner/session_file_locator_test.go`
- signature: `func TestRelocateSessionFileClaudePreservesProjectHashDirectory(t *testing.T)`
- expected input:
  - source path `.claude/projects/source-hash/claude-real-1.jsonl`
  - target account home
  - call `RelocateSessionFile(ProviderClaude, src, targetHome, "claude-real-1", "/repo")`
- expected output:
  - destination path `.claude/projects/source-hash/claude-real-1.jsonl`
  - file bytes equal source bytes
- covers: `DOD-29`, `DOD-30`

### Cross-Account Preparation Tests

#### `[done] TS-024` Same-account resume skips relocation

- target file: `apps/local-runner/internal/runner/interactive_handlers_test.go`
- signature: `func TestPrepareCrossAccountResumeSkippedForSameAccount(t *testing.T)`
- expected input:
  - reconstructed run has `providerAccountID:"acct-a"`
  - service `activeAccountID:"acct-a"`
  - call `resumeRun("run-1")`
- expected output:
  - no call to file relocation helper
  - handle returned successfully
  - persisted provider account remains `acct-a`
- covers: `DOD-33`

#### `[done] TS-025` Cross-account missing source file maps to session_unavailable

- target file: `apps/local-runner/internal/runner/interactive_handlers_test.go`
- signature: `func TestPrepareCrossAccountResumeMissingSourceFile(t *testing.T)`
- expected input:
  - persisted run account `acct-a`
  - active account `acct-b`
  - source account home exists but has no session file
  - call `resumeRun("run-1")`
- expected output:
  - API error status `409`
  - code `session_unavailable`
  - message `session data not found on this machine`
  - run remains visible in history store
- covers: `DOD-24`, `DOD-32`, `DOD-38`, `DOD-78`

#### `[done] TS-026` Cross-account active account not signed in maps to account_not_signed_in

- target file: `apps/local-runner/internal/runner/interactive_handlers_test.go`
- signature: `func TestPrepareCrossAccountResumeActiveAccountNotSignedIn(t *testing.T)`
- expected input:
  - source session file exists under account A
  - target account B home exists but auth check returns false
  - call `resumeRun("run-1")`
- expected output:
  - API error status `409`
  - code `account_not_signed_in`
  - user message says active account is not signed in
  - no relocation file is written
- covers: `DOD-25`, `DOD-26`, `DOD-79`

#### `[done] TS-027` Cross-account successful relocation re-points run

- target file: `apps/local-runner/internal/runner/interactive_handlers_test.go`
- signature: `func TestPrepareCrossAccountResumeRelocatesAndRepointsRun(t *testing.T)`
- expected input:
  - store has `run-1` with `ProviderAccountID:"acct-a"`, `ProviderSessionID:"sess-1"`
  - active account `acct-b`
  - source file exists, target auth check passes
  - call `resumeRun("run-1")`
- expected output:
  - session file copied into account B home
  - `s.runs["run-1"].providerAccountID == "acct-b"`
  - persisted session for `run-1` has `ProviderAccountID == "acct-b"`
  - `RunID` remains `run-1`
- covers: `DOD-34`, `DOD-35`, `DOD-36`, `DOD-37`, `DOD-80`

#### `[done] TS-028` Cross-account relocation failure keeps run visible

- target file: `apps/local-runner/internal/runner/interactive_handlers_test.go`
- signature: `func TestPrepareCrossAccountResumeRelocationFailureKeepsHistoryVisible(t *testing.T)`
- expected input:
  - project history has `run-1`
  - relocation helper returns error
  - call `resumeRun("run-1")`, then `projectRunHistory("project-1")`
- expected output:
  - resume returns typed conflict
  - history still contains `run-1`
  - no replacement run id created
- covers: `DOD-38`, `DOD-39`

### Claude Resume Tests

#### `TS-029` Restored Claude run seeds real session before turn

- target file: `apps/local-runner/internal/runner/claude_adapter_test.go`
- signature: `func TestRestoredClaudeRunSeedsPoolWithRealSessionBeforeTurn(t *testing.T)`
- expected input:
  - reconstructed run has `providerSessionID:"claude-real-1"`, `realProviderSessionID:"claude-real-1"`, `resumedFromDisk:true`
  - call start-turn path for a follow-up prompt
- expected output:
  - Claude pool has real mapping before spawn
  - generated args include `--resume claude-real-1`
- covers: `DOD-40`, `DOD-41`

#### `TS-030` Claude resume env uses active account home

- target file: `apps/local-runner/internal/runner/claude_adapter_test.go`
- signature: `func TestClaudeResumeUsesActiveAccountHomeEnvironment(t *testing.T)`
- expected input:
  - active account home `/tmp/claude-b`
  - restored run originally from account A but already relocated
  - fake spawn records env and cwd
- expected output:
  - env contains expected Claude config/home variables for `/tmp/claude-b`
  - cwd equals restored run working directory
- covers: `DOD-42`

#### `TS-031` Claude same-account post-restart e2e is opt-in

- target file: `apps/local-runner/internal/runner/claude_resume_e2e_test.go`
- signature: `func TestClaudePostRestartResumeEndToEnd(t *testing.T)`
- expected input:
  - env `FLOWPILOT_CLAUDE_RESUME_E2E=1`
  - valid Claude auth
  - create chat, persist session, recreate service, call resume and send follow-up
- expected output:
  - test skips when env is not set
  - when enabled, follow-up completes and provider emits a final assistant message
- covers: `DOD-43`, `DOD-87`, `DOD-88`

### Codex Resume Tests

#### `[done] TS-032` Fresh Codex run still uses app-server adapter

- target file: `apps/local-runner/internal/runner/codex_resume_process_test.go`
- signature: `func TestCodexFreshRunUsesAppServerPath(t *testing.T)`
- expected input:
  - non-restored Codex chat run
  - fake app-server adapter and fake CLI resume runner
  - send a first turn
- expected output:
  - app-server adapter called once
  - CLI resume runner not called
- covers: `DOD-45`

#### `[done] TS-033` Restored Codex run uses CLI resume path

- target file: `apps/local-runner/internal/runner/codex_resume_process_test.go`
- signature: `func TestCodexRestoredRunUsesCLIResumePath(t *testing.T)`
- expected input:
  - restored Codex chat run with `ProviderSessionID:"rollout-abc"`, `resumedFromDisk:true`
  - follow-up prompt `"continue"`
  - fake command runner records argv, env, cwd
- expected output:
  - argv contains `codex exec resume rollout-abc`
  - env contains `CODEX_HOME=<activeAccountHome>`
  - cwd equals run working directory
  - app-server adapter not called
- covers: `DOD-46`, `DOD-47`, `DOD-48`, `DOD-82`

#### `[done] TS-034` Codex resume command includes yolo-derived posture

- target file: `apps/local-runner/internal/runner/codex_resume_process_test.go`
- signature: `func TestCodexResumeCommandUsesYoloDerivedSandboxAndApproval(t *testing.T)`
- expected input:
  - restored Codex run with yolo mode off and on table cases
  - fake command runner captures args
- expected output:
  - each case includes the expected sandbox and approval policy config values from existing yolo helper behavior
- covers: `DOD-49`

#### `[done] TS-035` Codex resume stdout maps final response

- target file: `apps/local-runner/internal/runner/codex_resume_process_test.go`
- signature: `func TestCodexResumeStdoutMapsFinalAssistantMessage(t *testing.T)`
- expected input:
  - fake command runner returns stdout `"assistant final text\n"`
  - bridge records emitted events
- expected output:
  - emits one assistant message completed event with text `"assistant final text"`
  - emits one turn completed event
  - event run id and provider key match request
- covers: `DOD-50`, `DOD-51`

#### `[done] TS-036` Codex resume command failure maps turn failure

- target file: `apps/local-runner/internal/runner/codex_resume_process_test.go`
- signature: `func TestCodexResumeCommandFailureEmitsTurnFailed(t *testing.T)`
- expected input:
  - fake command runner exits non-zero with stderr `"auth failed"`
  - bridge records events
- expected output:
  - returns error or emits turn failed according to final process contract
  - no assistant completed event is emitted
  - error text does not include session file contents
- covers: `DOD-31`, `DOD-38`, `DOD-50`

#### `[done] TS-037` Codex same-account post-restart e2e is opt-in

- target file: `apps/local-runner/internal/runner/codex_resume_e2e_test.go`
- signature: `func TestCodexPostRestartResumeEndToEnd(t *testing.T)`
- expected input:
  - env `FLOWPILOT_CODEX_RESUME_E2E=1`
  - valid Codex auth
  - create chat, persist rollout id, recreate service, call resume and send follow-up
- expected output:
  - test skips when env is not set
  - when enabled, `codex exec resume` completes and emits assistant output
- covers: `DOD-52`, `DOD-87`, `DOD-88`

#### `[done] TS-038` Codex cross-account portability e2e is opt-in

- target file: `apps/local-runner/internal/runner/codex_resume_e2e_test.go`
- signature: `func TestCodexCrossAccountResumeEndToEnd(t *testing.T)`
- expected input:
  - env `FLOWPILOT_CODEX_CROSS_ACCOUNT_RESUME_E2E=1`
  - two valid Codex account homes
  - rollout created in account A and resumed under account B
- expected output:
  - test skips when env is not set
  - relocated rollout resumes under account B
  - run id remains unchanged
- covers: `DOD-53`, `DOD-87`, `DOD-90`

### Desktop State And UI Tests

#### `[done] TS-039` openHistoryRun handles resumable post-restart run

- target file: `apps/desktop-flowpilot/src/state/store.test.ts`
- signature: `it("openHistoryRun opens a reconstructed history run", async () => { ... })`
- expected input:
  - fake client `resumeRun("run-1")` resolves `{runId:"run-1", providerSessionId:"sess-1", providerKey:"codex", status:"completed", stepId:"chat-run-1"}`
  - fake stream returns completion
  - store has history item for `run-1`
- expected output:
  - store `runId == "run-1"`
  - `activeStepId == "chat-run-1"`
  - selected provider becomes `codex`
  - timeline is rebuilt from stream
- covers: `DOD-56`, `DOD-61`

#### `[done] TS-040` openHistoryRun greyouts typed resume error

- target file: `apps/desktop-flowpilot/src/state/store.test.ts`
- signature: `it("openHistoryRun marks item unavailable on typed resume errors", async () => { ... })`
- expected input:
  - fake client `resumeRun("run-1")` rejects API error `{code:"session_unavailable", message:"session data not found on this machine"}`
  - current store has active run `run-current`
- expected output:
  - current `runId` stays `run-current`
  - history item `run-1` gains disabled/unavailable state or equivalent store field
  - stored reason equals server message
  - timeline is not cleared
- covers: `DOD-58`, `DOD-59`, `DOD-60`, `DOD-83`

#### `[done] TS-041` openHistoryRun greyouts account_not_signed_in

- target file: `apps/desktop-flowpilot/src/state/store.test.ts`
- signature: `it("openHistoryRun marks item unavailable when active account is not signed in", async () => { ... })`
- expected input:
  - fake client rejects `{code:"account_not_signed_in", message:"can't open - the active account isn't signed in"}`
- expected output:
  - item disabled/unavailable reason matches server message
  - no navigation happens
- covers: `DOD-26`, `DOD-58`, `DOD-59`, `DOD-91`

#### `[done] TS-042` Navigator renders unified history without account UI

- target file: `apps/desktop-flowpilot/src/components/Navigator.test.tsx`
- signature: `it("renders unified history without account labels or grouping", () => { ... })`
- expected input:
  - history items from Codex and Claude, originally different accounts, but no account fields in UI contract
- expected output:
  - all items render in one list
  - no account group heading, account badge, or account filter control appears
- covers: `DOD-54`, `DOD-55`

#### `TS-043` Navigator renders disabled history reason

- target file: `apps/desktop-flowpilot/src/components/Navigator.test.tsx`
- signature: `it("renders unavailable history item disabled with reason", () => { ... })`
- expected input:
  - history item state includes unavailable reason `"session data not found on this machine"`
- expected output:
  - item has disabled/aria-disabled styling or disabled click target
  - tooltip/title or equivalent contains reason
- covers: `DOD-58`, `DOD-59`, `DOD-60`

### Compatibility Script Tests

#### `TS-044` quicktest detects Codex exec resume help

- target file: `scripts/quicktest.tests.ps1` or existing script test harness
- signature: `It "detects Codex exec resume CLI surface"`
- expected input:
  - fake `codex exec resume --help` output containing `SESSION_ID` and `PROMPT`
- expected output:
  - quicktest marks Codex session portability CLI surface as pass
- covers: `DOD-63`, `DOD-64`

#### `TS-045` quicktest fails Codex exec resume when help shape changes

- target file: `scripts/quicktest.tests.ps1` or existing script test harness
- signature: `It "fails Codex resume canary when CLI surface changes"`
- expected input:
  - fake help output missing `SESSION_ID`
- expected output:
  - quicktest marks the session portability section as fail or warning according to existing quicktest severity style
- covers: `DOD-64`

#### `TS-046` quicktest detects account fields in Codex rollout metadata

- target file: `scripts/quicktest.tests.ps1` or existing script test harness
- signature: `It "warns if Codex rollout metadata contains account scoped fields"`
- expected input:
  - fake rollout session meta JSON with key `account_id` or `auth`
- expected output:
  - quicktest fails or warns that portability contract changed
- covers: `DOD-65`

#### `TS-047` quicktest accepts account-agnostic Codex rollout metadata

- target file: `scripts/quicktest.tests.ps1` or existing script test harness
- signature: `It "passes account agnostic Codex rollout metadata"`
- expected input:
  - fake rollout session meta JSON with keys `id,timestamp,cwd,originator,cli_version,source,model_provider,base_instructions,dynamic_tools`
- expected output:
  - quicktest marks rollout metadata canary as pass
- covers: `DOD-65`

#### `TS-048` quicktest keeps compat config baseline behavior

- target file: `scripts/quicktest.tests.ps1` or existing script test harness
- signature: `It "uses compat-config baseline while adding portability canaries"`
- expected input:
  - temp `.flowpilot/settings/compat-config.json` with custom Codex/Claude tested versions
  - fake installed versions matching custom values
- expected output:
  - existing version baseline checks still use config values
  - new portability section does not reset or ignore config
- covers: `DOD-67`

### Manual And Opt-In E2E Test Signatures

#### `[done] TS-049` Manual same-account post-restart resume

- target file: `requirements/08-Task/todo/Task-071-Cross-Account-Chat-Resume-DOD-Checklist.md`
- signature: `Manual: Same-account post-restart resume`
- expected input:
  - signed-in provider account
  - start chat, complete one turn, restart app/runner, click history item, send follow-up
- expected output:
  - no `run_not_found`
  - same `run_id` opens
  - provider completes follow-up
- covers: `DOD-88`

#### `TS-050` Manual missing provider file greyout

- target file: `requirements/08-Task/todo/Task-071-Cross-Account-Chat-Resume-DOD-Checklist.md`
- signature: `Manual: Missing provider session file greyout`
- expected input:
  - history run with valid `sessions.ndjson`
  - provider session file moved/deleted
  - restart app/runner and click history item
- expected output:
  - item stays visible
  - item is disabled/greyed-out with `session data not found on this machine`
  - no crash and no timeline wipe
- covers: `DOD-89`

#### `TS-051` Manual active account not signed in greyout

- target file: `requirements/08-Task/todo/Task-071-Cross-Account-Chat-Resume-DOD-Checklist.md`
- signature: `Manual: Active account not signed in greyout`
- expected input:
  - run created under account A
  - switch to account B that has home but invalid/missing auth
  - click account A history item
- expected output:
  - greyout reason says active account is not signed in
  - no relocation occurs
- covers: `DOD-91`

#### `[done] TS-052` Manual history display after restart

- target file: `requirements/08-Task/todo/Task-071-Cross-Account-Chat-Resume-DOD-Checklist.md`
- signature: `Manual: History display survives restart`
- expected input:
  - multiple chat runs in one project
  - restart app/runner
  - open history
- expected output:
  - history order follows `updatedAt` descending
  - last prompt/message display fields remain present
  - no account grouping or filtering
- covers: `DOD-54`, `DOD-55`, `DOD-92`

#### `[done] TS-053` Manual no provider file content in logs

- target file: `requirements/08-Task/todo/Task-071-Cross-Account-Chat-Resume-DOD-Checklist.md`
- signature: `Manual: Provider session contents are not logged`
- expected input:
  - run through same-account resume, missing-file failure, and cross-account relocation attempt
  - inspect runner logs
- expected output:
  - logs may include run id, provider key, error code, and file path where acceptable
  - logs do not include conversation text or raw JSONL contents
- covers: `DOD-31`, `DOD-93`

#### `TS-054` Manual workflow run remains out of scope

- target file: `requirements/08-Task/todo/Task-071-Cross-Account-Chat-Resume-DOD-Checklist.md`
- signature: `Manual: Workflow run resume is not silently enabled`
- expected input:
  - persisted workflow run history item after restart
  - click workflow run history item
- expected output:
  - workflow run does not use chat reconstruction path
  - user-safe unsupported behavior or existing workflow behavior occurs
  - no fake chat step is attached to workflow run
- covers: `DOD-17`, `DOD-69`

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/local_file_session_store_test.go`
  - `apps/local-runner/internal/runner/interactive_service_test.go`
  - `apps/local-runner/internal/runner/interactive_handlers_test.go`
  - `apps/local-runner/internal/runner/session_file_locator_test.go`
  - `apps/local-runner/internal/runner/claude_adapter_test.go`
  - `apps/local-runner/internal/runner/codex_resume_process_test.go`
  - `apps/local-runner/internal/runner/claude_resume_e2e_test.go`
  - `apps/local-runner/internal/runner/codex_resume_e2e_test.go`
  - `apps/desktop-flowpilot/src/state/store.test.ts`
  - `apps/desktop-flowpilot/src/components/Navigator.test.tsx`
  - `scripts/quicktest.tests.ps1`
- modules:
  - `local-runner`
  - `desktop-flowpilot`
  - compatibility scripts
- routes:
  - `POST /client/workflow-runs/{runId}/resume`
  - `GET /client/projects/{projectId}/workflow-runs`
- tables:
  - `workflow_provider_sessions` only for Supabase reader parity tests

## 6. Acceptance Check

- All non-manual test signatures from `TS-001` through `TS-048` are implemented or explicitly deferred with reason.
- Manual/e2e signatures from `TS-049` through `TS-054` are documented in completion notes with pass/fail/skip status.
- Normal `go test` does not require real provider tokens, real Google Drive, or live network provider calls.
- Test failures clearly identify whether the break is persistence, reconstruction, relocation, provider resume, desktop greyout, or compatibility canary.

## 7. Out of Scope

- Full cross-PC Google Drive chat sync tests.
- Supabase-only chat sync tests from Task-057.
- Real provider API/token tests in the default test suite.
- Visual pixel-level desktop QA.
- Implementing the production feature code in this document.

## 8. Completion Notes

- result:
  - Implemented and verified in focused automated suites:
    - `TS-001` to `TS-021` (all non-deferred, non-blocked runner-level signatures)
    - `TS-022` to `TS-029`
    - `TS-032` to `TS-036`
    - `TS-039` to `TS-041`
  - Supporting test files now present:
    - `apps/local-runner/internal/runner/cross_account_resume_test.go` (TS-011/013/015-017/019/020 added 2026-06-18)
    - `apps/local-runner/internal/runner/codex_resume_process_test.go`
    - `apps/local-runner/internal/runner/supabase_workflow_store_test.go`
    - `apps/local-runner/internal/runner/claude_adapter_test.go` (TS-008, TS-029 added)
    - `apps/desktop-flowpilot/src/state/store.test.ts`
  - Verified commands:
    - `go test ./internal/runner/... -run 'TestResumeRunReconstructsChatRunSeedsChatStep|TestResumeRunMissingPersistedSessionReturnsRunNotFound|TestResolveAccountHomeExplicitAccount|TestResolveAccountHomeDefaultAccountFallback|TestResolveAccountHomeMissingAccount|TestLocateSessionFileCodexMissing|TestLocateSessionFileClaudeFindsProjectSession'` — 7 passed
    - `go test ./internal/runner/... -run 'TestClaudeAdapterPersistsRealSessionIDForResume|TestRestoredClaudeRunSeedsPoolWithRealSessionBeforeTurn'` — 2 passed
    - `npx tsx --test src/state/store.test.ts src/state/timelineReducer.test.ts src/lib/normalizeImage.test.ts`
    - `npm run typecheck`
- follow-ups:
  - Explicitly deferred signatures:
    - `TS-030`, `TS-031`
      - reason: Claude cross-account env / opt-in e2e requires real Claude auth or 2nd account; outside 09-IG MVP scope.
    - `TS-037`, `TS-038`
      - reason: real-provider opt-in e2e harness was not added in this task; normal `go test` remains token-free by design.
    - `TS-042`, `TS-043`
      - reason: no active renderer/component test harness exists in this repo for `Navigator.tsx`; state-level behavior is covered via `store.test.ts`.
    - `TS-044` to `TS-048`
      - reason: compatibility coverage was added as `scripts/quicktest.ps1` canaries, not as a separate PowerShell/Pester automated test harness.
  - Manual/e2e status:
    - `TS-049` skip
    - `TS-050` skip
    - `TS-051` skip
    - `TS-052` skip
    - `TS-053` skip
    - `TS-054` skip
      - reason: these require manual/provider-auth validation and were not executed in this coding pass.
- upstream docs updated:
  - `requirements/08-Task/todo/Task-071-Cross-Account-Chat-Resume-DOD-Checklist.md`
  - `requirements/08-Task/todo/Task-072-Cross-Account-Chat-Resume-Test-Signatures.md`
