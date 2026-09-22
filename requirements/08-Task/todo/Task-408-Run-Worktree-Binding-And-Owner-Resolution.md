# Task-408: Run Worktree Binding + Owner Resolution (CP-71 P-2, P-2b)

## Metadata

- Document ID: `Task-408`
- Title: `Run Worktree Binding — StartRunInput, Owner Key, Leg Inheritance, Run Record`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-71](../../07-Coding-Plan/todo/CP-71-Run-Worktree-Isolation.md) `P-2/P-2b`, [SD-27](../../06-System-Tech-Design/SD-27-Run-Worktree-Isolation.md) `D-4b/D-5/D-6/D-8/D-9`
- Child Documents: `None`
- Related Documents: [Task-407](./Task-407-Shared-Worktree-Manager-Package.md), [CP-59](../../07-Coding-Plan/done/CP-59-Chat-Ssot-Continuous-Cross-Provider-Chat.md)
- Replaces: `None`
- Tags: `worktree, runner, run-record, chat-ssot, go`
- Feature Keys: `run-worktree`

## AI Quick View

### Summary

- Wire `worktree:true` through `POST /client/workflow-runs` (`X-Client: desktop|tui` only, else `403 worktree_client_forbidden`), provision the worktree at `createRun`, point provider cwd at it, and persist the binding on the run record.
- Owner key (`D-8`): `chatId` for chat runs — new legs inherit the chat's live binding (resident `s.runs` → `ListProviderSessionsByChat` → `D-7` validate); parent flow `runId` for flow runs. Base `HEAD` captured at `createRun` for all run kinds.
- `worktreeEnabled` persisted per run record; per-chat restore derives from the newest run (D-9); toggle-off with a live binding is rejected.

### Current Ask

- Implement the binding end-to-end in the runner: input field, owner resolution + leg inheritance, provision, cwd wiring, run-record fields, snapshot/history view fields.

### Key Decisions

- `T-1` `worktreeOwnerID` resolution happens in `createRun` alongside `resolveChatIdentity` (same resolve-before-lock pattern); chat runs with an inherited binding skip `Manager.Create`.
- `T-2` Provider cwd = worktree path via the existing `workspaceCwd`/`StartRunInput.Cwd` channel — no adapter changes.
- `T-3` `worktreeEnabled` on the run record; restore rule = newest run in chat.

### Constraints

- `worktree:false`/absent → byte-for-byte today's behavior (`SS-23 AC-8`).
- No Supabase fields; `sessions.ndjson`/`ProviderSessionState` additions are `omitempty`.
- Additive tests only; old suite green; Claude/Codex/Grok parity — cwd-only change, evidence expected.

### Open Questions

- `None`

### Source Refs

- `SD-27 D-4b..D-9`, §5 data model, §6 contracts; `interactive_handlers.go` `handleStartRun`/`createRun` (~line 895-914 identity resolution), `provider_event.go` `StartRunInput`, `chat_ssot.go` `resolveChatIdentity`, `local_file_session_store.go`, `interactive_resume.go:888` (`workspaceCwd` rehydrate), `provider_registry.go` (`req.Cwd` honor sites).

## 1. Goal

An opted-in run starts with a provisioned worktree, a persisted owner-keyed binding, and provider cwd pointed inside it — for both chat legs and flow runs.

## 2. Parent Links

- coding plan: `CP-71 P-2, P-2b`
- tech design: `SD-27 D-4b..D-9`
- system spec: `SS-23 AC-1/AC-2/AC-7/AC-8/AC-9/AC-10, BR-1/BR-3`
- specific upstream ids: `Task-407` (manager), `CP-59/SD-26` (chat legs)

## 3. Trigger

Manager exists (Task-407); this task binds it to the run lifecycle at the single `createRun` entry shared by chat and flow.

## 4. Exact Change

- `T-1` `StartRunInput.Worktree bool` + `X-Client` gate; `createRun` → `resolveWorktreeOwner` → validate git repo (`worktree_unavailable` otherwise) → inherit-or-create via `internal/worktree` → set `rs.workspaceCwd` = worktree path → persist binding fields.
- `T-2` Run record/session fields (`omitempty`): `worktreeOwnerID`, `worktreePath`, `worktreeBranch`, `worktreeBaseCommit`, `worktreeSlug`, `worktreeState`, `worktreeEnabled`; persisted in `ProviderSessionState` → `sessions.ndjson`; rehydrated in `interactive_resume.go`.
- `T-3` `runSnapshotView.worktree` block + `runHistoryItem.{worktreeState,worktreeSlug}` for badges.
- `T-4` Toggle-off guard: `worktree:false` on a chat whose newest leg has a live binding → `409 worktree_binding_active` with message.

## 5. Touched Areas

- files: `internal/runner/{provider_event.go, interactive_handlers.go, interactive_service.go, interactive_resume.go, local_file_session_store.go}` (+ maybe `chat_ssot.go` helper), new `run_worktree.go`.
- modules: `internal/runner`, `internal/worktree` (consumer).
- routes: `POST /client/workflow-runs` (existing).
- tables: none.

## 6. Code Guide Signatures

```go
// apps/local-runner/internal/runner/run_worktree.go (new)
type worktreeBinding struct {
    OwnerID    string `json:"worktreeOwnerId,omitempty"`
    Path       string `json:"worktreePath,omitempty"`
    Branch     string `json:"worktreeBranch,omitempty"`
    BaseCommit string `json:"worktreeBaseCommit,omitempty"`
    Slug       string `json:"worktreeSlug,omitempty"`
    State      string `json:"worktreeState,omitempty"`  // none|active|merge_pending|merged|kept_branch|discarded|lost
    Enabled    bool   `json:"worktreeEnabled,omitempty"`
}

// resolve-before-lock, mirrors resolveChatIdentity (chat_ssot.go:97)
func (s *InteractiveService) resolveWorktreeOwner(in StartRunInput, chatID string, runKind string) (ownerID string, inherited *worktreeBinding, err *apiErr)

func (s *InteractiveService) provisionRunWorktree(rs *interactiveRun, ownerID string) *apiErr // T-1
func (s *InteractiveService) inheritChatWorktree(chatID string) (*worktreeBinding, bool)      // T-1: s.runs → ListProviderSessionsByChat
func (s *InteractiveService) validateWorktreeBindingLocked(b *worktreeBinding) bool           // D-7
```

```go
// provider_event.go — additive field
type StartRunInput struct { /* … */ Worktree bool `json:"worktree,omitempty"` }

// interactive_handlers.go — views additive
type runSnapshotView struct { /* … */ Worktree *worktreeView `json:"worktree,omitempty"` }
type runHistoryItem struct { /* … */ WorktreeState, WorktreeSlug string /* omitempty */ }
```

## 7. Test Signatures

- `TestStartRun_WorktreeProvisionsAndSetsCwd` — `worktree:true` → binding persisted, `rs.workspaceCwd` = `<root>/.flowpilot/worktrees/<chatId>`, provider receives that cwd.
- `TestStartRun_WorktreeClientGate` — `X-Client` absent/`admin` + `worktree:true` → 403.
- `TestStartRun_WorktreeNonGitRepo` — → `worktree_unavailable`; no dir created.
- `TestStartRun_LegInheritsChatWorktree` — leg2 (same `chatId`, `SwitchFromRunID`) reuses binding; no second `git worktree add`; cwd = same path; files from leg1 visible.
- `TestStartRun_LegInheritsAfterRestart` — fresh service over persisted sessions; new leg inherits via `ListProviderSessionsByChat`.
- `TestStartRun_ToggleOffWithLiveBindingRejected` — 409 `worktree_binding_active`.
- `TestStartRun_FlowRunOwnerIsSelf` — harness/vibe run → `worktreeOwnerID == runID`; spawned child inherits `parent.workspaceCwd` (existing propagation, assert observed cwd).
- `TestStartRun_WorktreeOffByteParity` — toggle absent → no `worktree*` fields, cwd unchanged.
- `TestRunSnapshot_IncludesWorktreeBlock`; `TestRunHistory_WorktreeFields`.

## 8. Acceptance Check

- Chat run with toggle on: provider process cwd inside worktree; edit lands in worktree, not main.
- Switch provider in that chat: new leg same cwd, prior edits still there.
- Flow run with toggle on: flow + children write inside worktree.
- Toggle off: identical to today's run.

## 9. Out of Scope

- Toggle UI (Task-409); merge-back card/endpoint (Task-410); `lost` handling + GC (Task-411).

## 10. Definition of Done

- [ ] §6 signatures landed; §7 tests exist, green, additive-only.
- [ ] Old suite untouched/green — failure → STOP + report.
- [ ] Provider parity: cwd-only change; verified `req.Cwd` honor sites unchanged + at least one provider exercised, evidence in CA.
- [ ] `feature_key: run-worktree`; CA ledger entry; `detect_changes` clean.

## 11. Completion Notes

- result: `TBD`
- follow-ups: `TBD`
- upstream docs updated: `TBD`
