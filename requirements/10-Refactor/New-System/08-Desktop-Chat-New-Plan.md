# CHAT MODE

## 1. History

Task 056: have code, pending test
057: not execute
Sau khi done thi item history can co 1 state chi ra dang run hay da done hay dang wait approve
now only press history button then the history tab left side update
> Status legend: ✅ resolved (verified in code) · ⚠️ open gap (real, still broken) · ❓ open decision · 🔭 new feature (separate from History core)

### 1.1 Current model — ✅ resolved / verified in code

#### Persistence (luu history nhu nao)

- The **transcript** stays as provider-owned **JSONL on disk** (Codex thread log / Claude session log). FlowPilot does not copy it.
- A **pointer row** is persisted to Supabase `workflow_provider_sessions`, keyed by `(workflow_run_id, provider_key, working_directory)` → `provider_session_id` / `provider_thread_id`.
  - Codex: `provider_session_id == provider_thread_id == Codex thread ID`.
  - Claude: `provider_session_id == real Claude session_id (UUID)`, bound to `cwd`.
- Files: `supabase/migrations/20260615120000_add_workflow_provider_tables.sql` (table), `apps/local-runner/internal/runner/supabase_provider_session_store.go:90` (`UpsertSession`, on-conflict upsert).

#### Resume semantics

- Resume needs **both** the persisted row **and** the local JSONL log on **the same machine**. Different machine / missing log → resume fails.
- Resume is bound to `(provider, cwd, session_id)`, **not** to an account → any active account of the same provider can resume, as long as the local log is reachable. FlowPilot therefore **does not** persist an owning account on the session row.
- Resolves the note "Neu active acc nhung khac acc thi co tim thay local log khong?": **yes** — resume reads the local log, not the account's remote history.
  - ❓ Assumption to keep explicit: this holds only while providers don't validate the session server-side against the owning account. If they ever do, cross-account resume breaks.

#### How the list is built (show list nhu nao) — decision made

- We do **NOT** fan out to each provider's native list API (e.g. Codex `thread/list`) and merge. That would mean N schemas + N sort/merge problems + history scoped to provider accounts instead of projects.
- Instead `projectRunHistory()` merges **in-memory active runs** + **persisted provider sessions**, sorted by `UpdatedAt` desc — project-scoped and provider-agnostic.
  - Endpoint: `GET /client/projects/{projectId}/workflow-runs` → `apps/local-runner/internal/runner/interactive_handlers.go:585` (`projectRunHistory`).
  - Desktop: `loadRunHistory` → `apps/desktop-flowpilot/src/state/store.ts:476`; renders in `RunStatus.tsx`.
- → The "thread/list vs supabase" question is **closed**: source of truth is the Supabase session rows.

### 1.2 ⚠️ Confirmed open gap — production empty History (BUG-060 F-2 / F-5)

The note "dang miss table workflow_provider_sessions trong migration?" is **literally fixed** (table exists in `20260615120000_add_workflow_provider_tables.sql`), but the real gap that causes "BUG: start 2 run, switch qua lai, press History show empty" is **still open in production**:

- `projectRunHistory` rehydrates persisted sessions only via a type assertion to the `SessionHistoryReader` interface:
  ```go
  // interactive_handlers.go:612
  if reader, ok := s.workflowStore.(SessionHistoryReader); ok { ... }
  ```
- `SessionHistoryReader.ListProviderSessionsByProject` is implemented **only by `fakeWorkflowStore`** (`workflow_store.go:195`). The production `SupabaseWorkflowStore` does **not** implement it.
- → For Supabase the assertion is `false`, the read path falls back to **in-memory `s.runs` only**, so History **still empties** after any app-server / runner recreation.

**Truth about BUG-060 status:** marked _done_, but only dev/demo (fake store) is actually fixed — stale-response guard (`_historyLoadSeq`), error-vs-empty distinction, and passing regression test (`bug060_test.go`) all exercise the fake store. **The user-visible prod bug is NOT fixed.**

Action items (these are the actual fix, not "add a migration"):

- **F-2** — implement `ListProviderSessionsByProject` on `SupabaseWorkflowStore` (`supabase_workflow_store.go`), querying `workflow_provider_sessions` by project. → **[Task-056](../../08-Task/todo/Task-056-History-Supabase-Reader-Production-Fix.md)**
- **F-5** — confirm/verify `workflow_provider_sessions` migration is applied in the production Supabase project. → **[Task-056](../../08-Task/todo/Task-056-History-Supabase-Reader-Production-Fix.md)**
- Re-label BUG-060 as "dev-only fixed; prod pending F-2/F-5" until F-2 lands.

### 1.3 🔭 Separate follow-up items (not History-core — split into own tasks)

- **Kill process before timeout on project page** ("co ca vu kill process before timeout setting o page proj"): today there is only graceful `interrupt` (pause) and `terminate-session` (refuses on finished runs). A hard kill-before-timeout is **net-new**, not a History fix.
- **support model+reason in CHAT MODE**: model+reason is PASSED for Flow mode (see §3); extend into chat mode. Separate feature.
- **Live History updates** ("press history run need see update of runs"): History is a point-in-time fetch and does not live-update while a run progresses. Needs refresh-on-event or subscribing the history panel to the event stream.
- **Provider-aware slash lists** ("/skills list actually skill", "/agents list actually agent", "based on current provider"): enumerate the real skills/agents of the **currently selected provider**, not a static list. Provider-adapter work.
- **Cross-PC chat sync** — sync provider CLI session files (`~/.claude/`, `~/.codex/`) to Google Drive so history can be resumed on another PC. → **[Task-057](../../08-Task/todo/Task-057-Cross-PC-Provider-Chat-Sync.md)** (requires Task-056 first; portability test must pass before coding)

## 2. YOLO - PASSED

### 2.1.1 Codex

OK for both MCP + Command tool

### 2.1.2 CLAUDE

OK for both MCP + Command tool

## 3. model.skill.reason - PASSED

### 3.1 Codex

ok

### 3.2 Claude

ok

## 4 Image attached - PASSED

Done in Task-052

### Has bug

------------------------- later prompt can not see image attached in previous prompt

## 5. User Interaction (Structured Questions) - PASSED

Suggestion FORM show to user
Separate from approvals, FlowPilot can ask the user a structured question (confirm/options popup) through the same pause/resume bridge

- ask_user MCP tool — a FlowPilot-registered custom tool (not built-in) the model discovers via tools/list and may call → best-effort, can not make sure it asked users
- Workflow-driven — the ported Go state machine emits user_question_required directly at a defined step → deterministic (use for required asks).

Other input was not worked - Passed now

## 6. Show token/context - PASSED

Check for codex -> OK
Check for claude -> OK

Recorded in [Task-061: Desktop Chat Token Usage And Context Window](../../08-Task/done/Task-061-Desktop-Chat-Token-Usage-And-Context-Window.md)

Whenever once AI return response
-> Check current provider
-> See is it configured to visible in account active right bar or not
-> if yes, Check the current active acc of this provider
-> reload IT ONLY to refresh the token on acc UI

## 7. How to check change in feature of codex/claude - PASSED
Retest with codex/claude latest
Task 059- DONE

If affected to our app cause we call its functions

## 8. Ui of provider with icon - PASSED
update ui of provider selection
new rule: can not CHANGED provider after run started

## 9. Feature cancel current running chat turn - PASSED

## 10. Load long chat - PASSED
Task 062

## 11. skill attached - AI k doc dc ? sai folder?

## 12. Sync artiact for chat mode first

# Flow mode - Pending

model
skill
agen-multi auto interact with each other
reasong
yolo
user question list
MCP call
image attach
