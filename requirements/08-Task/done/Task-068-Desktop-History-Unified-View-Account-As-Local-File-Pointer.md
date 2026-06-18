# Task-068: Desktop History Unified View; Account ID As Local-File Pointer

## Metadata

- Document ID: `Task-068`
- Title: `Desktop History Unified View; Account ID As Local-File Pointer`
- Phase: `task`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-17`
- Last Updated: `2026-06-17`
- Parent Documents: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- Child Documents: `none`
- Related Documents: [09-IG: Cross-Account Chat Resume Implementation Guide](../../10-Refactor/New-System/09-Cross-Account-Chat-Resume-Implementation-Guide.md), [CA-098: Provider Session Portability Spike](../../../change-audit/CA-098-spike-provider-session-portability.md), [BUG-080: Desktop Run History Lost On App Restart No Supabase](../../09-BugFix/done/BUG-080-Desktop-Run-History-Lost-On-App-Restart-No-Supabase.md), [Task-067: Desktop Post-Restart Run Resume Via Provider Session Id](./Task-067-Desktop-Post-Restart-Run-Resume-Via-Provider-Session-Id.md), [Task-018: Auto Switch Account](./Task-018-Auto-Switch-Account.md), [CA-097: Fix Desktop Run History Lost On Restart No Supabase](../../../change-audit/CA-097-fix-desktop-run-history-lost-on-restart-no-supabase.md)
- Replaces: `none`
- Tags: `desktop, history, local-runner, accounts, sessions, ndjson, codex, claude`

## AI Quick View

### Summary

- The user does NOT want history grouped or filtered by provider account. The sidebar must show every chat (chat A, chat B, …) as one unified list regardless of which account started each.
- The account ID's only role is internal plumbing: it tells the runner WHICH local provider folder holds the session file for a run (account A → `…/slot-0/`, account B → `…/slot-1/`). It is a file-location pointer, never a UI display dimension.
- Today `sessions.ndjson` does NOT persist `ProviderAccountID` (it is on the in-memory `ProviderSessionState` but missing from `ndjsonSessionRecord`), so after restart the runner cannot know which local folder to read a session's files from.
- Forward capability (needs feasibility test, may split into its own task): continue a chat started under account A while account B is active. Mechanism is the same "relocate the session file into the active account's folder, then resume" used by cross-PC sync (Task-069).

### Current Ask

- Persist `ProviderAccountID` in `sessions.ndjson` purely so the runner can locate the correct local provider folder for each run after restart.
- Keep the history sidebar a single unified list — do NOT add account badges, account grouping, or account-based filtering.
- Make `provider_session_id` mutable per `run_id` (write the foundation for re-pointing a run to a new provider session — see Task-067 / Task-069), so a chat can later be continued under a different account or PC without losing its identity.
- Spike the feasibility of continuing chat A under account B (read-only investigation + manual test); record the verdict before building it.

### Key Decisions

- `T-1` History is unified. The desktop shows all runs for a project in one list; the active account does not filter or reorder the list. No "other account" badge.
- `T-2` `ProviderAccountID` is persisted to `ndjsonSessionRecord` as `provider_account_id` (`omitempty`) so the runner can resolve the correct local provider folder for each run on resume.
- `T-3` `run_id` is the durable identity; `provider_session_id` (and the `provider_account_id` that locates its files) are mutable per run. Continuing a chat under a different account re-points these fields while `run_id` stays fixed, so the user keeps seeing the same chat.
- `T-4` Continuing chat A under account B is NOT in the core slice. It is captured here as a forward capability with an explicit feasibility gate (Q-1). The core slice only persists the pointer and keeps the view unified.
- `T-5` `provider_session_id` reuse vs re-mint is provider-driven: a session file is local conversation state and the account is auth/billing for new turns. These are separable, so cross-account continuation should be a file-relocation + resume, not a re-auth of the original session.
- `T-6` Fallback policy (user decision 2026-06-17): if a chat cannot be continued under a different account (provider rejects the relocated file), it stays VISIBLE in history but greyed-out / disabled with a description ("can't open — created by a different account"). Not hidden, not history-injected. (Canonical greyout fallback owned by Task-067 T-4.)

### Constraints

- Do NOT group, badge, or filter history by account in the UI — the user explicitly wants one unified list.
- NDJSON schema change must be backwards-compatible — entries without `provider_account_id` load cleanly (empty string).
- Do not change account-switching logic itself (Task-018 owns that).
- Cross-account continuation must be verified by a manual test before any implementation — providers may reject a session file created under a different account.
- Session files are user-sensitive; never log their contents.

### Open Questions

- `Q-1` Can a chat started under account A be continued under account B? **Codex: CONFIRMED YES, end-to-end** ([CA-098](../../../change-audit/CA-098-spike-provider-session-portability.md), 2026-06-17) — account B's `codex` resumed account A's copied rollout by id (no index entry needed) and completed a new turn (model replied, full 28k-token prior context loaded, exit 0). The rollout has no account/auth fields. Only precondition: the target account must be validly logged in (verified by contrast — an account with an expired token loaded the file but failed the turn on auth). **Claude: still untested** (one account on the test machine).
- `Q-2` For Codex, the app-server is bound to one account/home at a time (see `SetActiveAccount`). To continue account A's thread under account B, must the rollout file be copied into B's home first, or can the app-server be pointed at a foreign rollout path? Confirm the least-invasive mechanism.
- `Q-3` For Claude, does `claude --resume <sessionId>` work when the session JSONL was created under a different account home, once the file is placed in the active account's `~/.claude/projects/<hash>/`? Confirm.
- `Q-4` When continuing under account B, do we COPY account A's session file into B's folder (duplicates data, simplest) or POINT B's process at A's file (no duplication, needs flag support)? Prefer copy unless duplication is a real concern.
- `Q-5` What `provider_account_id` is recorded for runs created while `activeAccountID == "default"` (before any explicit account selection)? Treat "default" as a normal account id for pointer purposes.

### Source Refs

- `apps/local-runner/internal/runner/local_file_session_store.go` — `ndjsonSessionRecord`, `sessionStateFromRecord`, `sessionRecordFrom`
- `apps/local-runner/internal/runner/interactive_handlers.go` — `runHistoryItem`, `projectRunHistory`, `createRun`
- `apps/local-runner/internal/runner/interactive_service.go` — `createRun` sets `rs.providerAccountID = s.activeAccountID`; `SetActiveAccount` (app-server account binding)
- `apps/local-runner/internal/cli/root.go` — `NextAccountHomePath` / account slot home paths (`/provider-accounts/allocate-slot`)
- `apps/desktop-flowpilot/src/components/Navigator.tsx` — history list render (must stay unified)

## 1. Goal

After restart, the history sidebar shows every chat for a project as one list, regardless of which provider account started each. Behind the scenes the runner knows which local folder each run's session files live in (via the persisted account id), which is the prerequisite for reading or resuming those files — including, as a follow-on capability, continuing a chat under a different account.

## 2. Parent Links

- coding plan: [CP-18](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- specific upstream ids: BUG-080 §7 F-2 (`ndjsonSessionRecord` schema), BUG-080 §7 F-5 (`runHistoryItem` mapping)

## 3. Trigger

BUG-080 created `sessions.ndjson` as the local session index but did not persist the account id. With multiple Codex/Claude accounts, each account's session files live in a different local folder. After restart the runner has no way to know which folder a given run's files are in — so it cannot read or resume them. The user's requirement is explicitly NOT to surface accounts in the UI (history stays unified); the account id is needed purely as an internal file-location pointer, plus it lays the groundwork for continuing a chat under a different account.

## 4. Exact Change

### Core slice — persist the pointer, keep the view unified

- `T-1` In `ndjsonSessionRecord`: add `ProviderAccountID string \`json:"provider_account_id,omitempty"\``.
- `T-2` In `sessionRecordFrom`: copy `s.ProviderAccountID` → `r.ProviderAccountID`.
- `T-3` In `sessionStateFromRecord`: copy `r.ProviderAccountID` → `s.ProviderAccountID`.
- `T-4` Confirm `projectRunHistory` continues to return ALL runs for a project unfiltered by account. No UI change in `Navigator.tsx` — explicitly do not add badges/grouping/filtering.
- `T-5` Make `UpsertProviderSession` overwrite `provider_session_id` and `provider_account_id` for an existing `run_id` (mutable mapping). NDJSON last-wins already supports this; verify the in-memory map updates the same keys rather than appending a second logical run.

### Forward capability — continue chat A under account B (gated by Q-1)

- `T-6` (spike, not implementation) Run the Q-1 manual test for Codex and Claude. Record whether a relocated session file resumes under a different account.
- `T-7` (only if T-6 passes) Add a runner path that, on resume of a run whose `provider_account_id` differs from the active account, relocates the session file into the active account's folder (Q-4 copy vs point) and resumes, then re-points `provider_account_id` (and `provider_session_id` if re-minted) for that `run_id`.
- `T-8` If relocation/resume fails (provider rejects the foreign-account file), the desktop shows that history item greyed-out / disabled with a description ("can't open — created by a different account"). The chat stays visible; only continue is disabled. (Shared greyout fallback — Task-067 T-4.)

### Definition Of Done

- [x] `DOD-068-001` `provider_account_id` is persisted in local `sessions.ndjson` with backward-compatible `omitempty`.
- [x] `DOD-068-002` Legacy NDJSON records without `provider_account_id` still load cleanly.
- [x] `DOD-068-003` Last-wins upsert re-points `provider_session_id` and `provider_account_id` for an existing `run_id`.
- [x] `DOD-068-004` Project history remains a unified list and is not filtered by account.
- [x] `DOD-068-005` Cross-account resume relocates provider session files into the active account home when feasible.
- [x] `DOD-068-006` Cross-account resume re-points persisted `provider_account_id` after successful relocation.
- [x] `DOD-068-007` Cross-account failure leaves the history item visible and returns a typed unavailable error.
- [x] `DOD-068-008` Same-account resume does not mutate the stored provider-account pointer.
- [x] `DOD-068-009` Codex feasibility is recorded as confirmed in CA-098 and reflected in implementation/tests.
- [ ] `DOD-068-010` Claude cross-account feasibility is validated end-to-end.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/local_file_session_store.go` — `ndjsonSessionRecord`, conversion helpers, mutable upsert (T-1–T-3, T-5)
  - `apps/local-runner/internal/runner/interactive_handlers.go` — confirm unified `projectRunHistory` (T-4)
  - `apps/desktop-flowpilot/src/components/Navigator.tsx` — no change (guard against accidental account UI)
  - (T-7 only) provider adapters + `resumeRun` for cross-account relocation
- modules: `local-runner`, `desktop-flowpilot`
- routes: `GET /client/projects/{projectId}/workflow-runs` (response unchanged in shape for the UI; `provider_account_id` stays server-side plumbing)

## 6. Acceptance Check

- After restart with two accounts configured, the history sidebar shows chat A and chat B together in one list, with no account labels or filtering.
- The runner can resolve the correct local provider folder for each restored run (verifiable via a resume attempt reaching the right folder, or a unit test on the pointer round-trip).
- An old `sessions.ndjson` entry without `provider_account_id` loads without error.
- `go test ./internal/runner/...` passes, including an updated restart round-trip test asserting `ProviderAccountID` survives.
- (If T-7 built) A chat started under account A can be continued while account B is active, and the run keeps its `run_id` and history.

## 7. Out of Scope

- Account badges, grouping, or account-based filtering in the history UI — explicitly excluded.
- Account-switching logic (Task-018).
- Cross-PC sync (Task-069) — though it shares the file-relocation mechanism.
- Supabase-backed history (`workflow_provider_sessions` already stores `provider_account_id`).
- Building T-7 before the Q-1 feasibility test passes.

## 8. Completion Notes

- result:
  - The local-file provider-account pointer is persisted and round-trips through restart.
  - Unified history behavior is preserved while the runner uses `provider_account_id` internally for cross-account session relocation and re-pointing.
- follow-ups:
  - If T-6 confirms feasibility, T-7 (cross-account continuation) may be promoted to its own task.
  - Re-pointing `run_id → provider_session_id` is shared with Task-067 (post-restart resume) and Task-069 (cross-PC); keep the mutation logic in one place.
- upstream docs updated:
  - Added inline DoD checklist with current implementation state.
