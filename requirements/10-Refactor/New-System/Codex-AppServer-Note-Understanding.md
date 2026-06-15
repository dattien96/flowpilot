# Note: Understanding FlowPilot Session Model With Codex App-Server

This note explains:

1. what `codex app-server` is in our architecture
2. how `cwd`, `thread`, `session`, and `run` relate
3. how a run is attached to a project
4. how resume works
5. how FlowPilot maps its concepts onto Codex app-server

## 1. Big Picture

`codex app-server` is not the FlowPilot product architecture.

It is the Codex runtime process that FlowPilot talks to through `CodexAdapter`.
FlowPilot still owns:

- workflow state
- run lifecycle
- prompt building
- approval policy
- event normalization
- persistence
- project/workspace mapping

High-level shape:

```text
Admin Web / Desktop Client
          |
          v
Go Runner (FlowPilot-owned backend)
  - WorkflowRuntime
  - ProviderRuntimeGateway
  - CodexAdapter
  - ProviderSessionStore
  - ProviderEventStore
          |
          v
1 shared Codex app-server process
  - thread A (cwd = project A)
  - thread B (cwd = project A)
  - thread C (cwd = project B)
```

## 2. Core Vocabulary

### Codex side

- `app-server`: one long-lived Codex runtime process
- `cwd`: working directory bound to a thread
- `thread`: a Codex chat/session identity with its own persisted history
- `turn`: one user message + assistant response cycle inside a thread

### FlowPilot side

- `project`: FlowPilot project/workspace record
- `run`: one FlowPilot workflow/chat execution instance
- `provider session`: FlowPilot's stored link to the provider-side conversation

For Codex specifically:

```text
FlowPilot project/workspace  -> Codex cwd
FlowPilot chat session       -> Codex thread
FlowPilot provider_session_id -> Codex threadId
FlowPilot run turn           -> Codex turn/start
```

## 3. The Main Mapping

```text
FlowPilot Project
  project_id = proj_123
  local_path = /Users/me/work/appA
          |
          v
FlowPilot Run
  run_id = run_456
  project_id = proj_123
  status = active
          |
          v
workflow_provider_sessions row
  workflow_run_id = run_456
  project_id = proj_123
  provider_key = codex
  provider_account_id = acct_codex_main
  working_directory = /Users/me/work/appA
  provider_session_id = thread_abc
  provider_thread_id  = thread_abc
          |
          v
Codex app-server
  thread.id = thread_abc
  cwd = /Users/me/work/appA
```

Meaning:

- the project tells FlowPilot which local path is the workspace
- the run belongs to that project
- the provider-session row binds that run to one Codex thread
- the Codex thread is created or resumed with that workspace path as `cwd`

## 4. One App-Server, Many Projects

The target architecture is not "one Codex process per project".

It is:

```text
1 active Codex account
        ->
1 shared codex app-server process
        ->
many threads
        ->
each thread carries its own cwd
```

Diagram:

```text
Runner
  |
  +-- shared codex app-server
        |
        +-- thread T1, cwd=/repo/mobile-app
        +-- thread T2, cwd=/repo/mobile-app
        +-- thread T3, cwd=/repo/admin-web
        +-- thread T4, cwd=/repo/local-runner
```

So:

- many projects can share one app-server
- one project can have many chats
- each chat is a separate Codex thread

## 5. What Is a Run?

A `run` is FlowPilot's workflow/chat execution record.

It is not the same thing as:

- the Codex app-server process
- a Codex thread
- a Codex turn

Think of it this way:

```text
run = FlowPilot execution container
thread = provider conversation identity
turn = one interaction inside that conversation
```

Typical sequence:

```text
1. User starts a run for project A
2. Runner resolves project A -> cwd /path/to/projectA
3. Runner creates or resumes a Codex thread for that run
4. Runner sends turn/start on that thread
5. Events stream back and are persisted on the run
6. Later the same run can reopen the same thread
```

## 6. How a Run Attaches to a Project

Attachment happens in two layers.

### Business layer

The FlowPilot run stores `project_id`.

That means:

- the run belongs to a known FlowPilot project
- the project supplies the canonical workspace path

### Runtime layer

The provider session stores `working_directory`.

That means:

- when the runner talks to Codex, it passes the project's path as `cwd`
- this is how Codex knows which files/repo the chat belongs to

So "this chat belongs to this project" is represented by:

```text
run.project_id
+ provider_session.working_directory
+ provider_session.provider_session_id(threadId)
```

## 7. Session, Thread, and Provider Session

These names are easy to confuse.

### Codex thread

This is the real conversation identity in Codex.

- has `thread.id`
- has persisted JSONL history on disk
- can be listed, read, resumed, archived

### FlowPilot provider session

This is our stored pointer to the provider conversation.

For Codex:

```text
provider_session_id == provider_thread_id == Codex thread.id
```

So FlowPilot is not inventing a second conversation identity for Codex. It is
storing Codex's thread id in our DB so we can reopen it later.

## 8. How Resume Works

Resume is not magic. It is a lookup + reattach flow.

### Resume flow

```text
1. Client asks runner to resume run R
2. Runner loads the provider-session row for run R
3. Runner gets:
   - provider_account_id
   - working_directory
   - provider_session_id (= threadId)
4. Runner verifies the active Codex account matches provider_account_id
5. Runner ensures the shared codex app-server is running for that account
6. Runner calls Codex thread/resume(threadId)
7. Codex reloads the thread history from its local JSONL log
8. Runner continues sending turns on that same thread
```

Diagram:

```text
POST /client/workflow-runs/{runId}/resume
          |
          v
FlowPilot Runner
  -> load workflow_provider_sessions by runId
  -> validate active account
  -> ensure shared app-server exists
  -> call thread/resume(threadId)
          |
          v
Codex app-server
  -> reopen stored thread history
  -> return resumed thread handle
```

## 9. What Resume Requires

Resume requires all of these:

1. stored `threadId`
2. same Codex account still active
3. same machine or same `CODEX_HOME` thread log available

Why:

- Codex stores thread history as local JSONL logs
- those logs live under the active account's `CODEX_HOME`
- `thread/resume` only works if the thread history is physically available there

So this is valid:

```text
same machine
+ same account
+ same CODEX_HOME
+ saved threadId
-> resume works
```

This is not guaranteed:

```text
different machine
or different account
or missing local thread log
-> resume may fail
```

## 10. Difference Between Reconnect and Resume

These are different cases.

### Reconnect

The client UI disconnects and reconnects, but the runner and thread are still
alive.

- rebuild stream from persisted event `seq`
- no need to reopen thread history

### Resume

The conversation must be reattached later, after runner restart or after time has
passed.

- load stored `threadId`
- call `thread/resume`
- continue on the same Codex thread

## 11. How FlowPilot Maps to Codex App-Server

The mapping is intentionally thin:

```text
FlowPilot project/workspace
  -> cwd

FlowPilot run
  -> operational container that owns status, events, approvals, persistence

FlowPilot provider session
  -> stored pointer to Codex thread

FlowPilot send turn
  -> Codex turn/start

FlowPilot reopen chat
  -> Codex thread/resume

FlowPilot list chats in a project
  -> Codex thread/list filtered by cwd

FlowPilot preview chat
  -> Codex thread/read
```

This is the key design choice:

- Codex owns thread lifecycle and thread persistence
- FlowPilot owns business logic, project/run mapping, and durable workflow state

## 12. Practical Example

```text
Project:
  project_id = android_app
  path = /work/android-app

Run:
  run_id = run_1001
  project_id = android_app

First start:
  runner -> thread/start(cwd=/work/android-app)
  codex returns threadId=th_01
  runner stores provider_session_id=th_01

Later:
  user opens run_1001 again
  runner loads th_01
  runner -> thread/resume(th_01)
  codex reloads history
  runner sends another turn on th_01
```

## 13. Final Mental Model

Use this compressed model:

```text
project = where
run     = FlowPilot execution record
thread  = Codex conversation identity
turn    = one message cycle
cwd     = project path attached to the thread
resume  = reopen the saved threadId under the same account/home
```

And the full stack:

```text
FlowPilot project
  -> FlowPilot run
  -> provider session row
  -> Codex thread
  -> Codex turns
```

## 14. Current Schema Gap In This Repo

Missing table thi sao work?
vu save history nhu nao?

Important: the refactor docs and the new runner store code refer to these
Supabase tables:

- `workflow_provider_sessions`
- `workflow_provider_events`
- `workflow_provider_approvals`
- `workflow_provider_questions`

But in the current repo migrations, those tables are not created yet.

What the repo migrations currently create is the older table:

- `workflow_run_sessions`

That older table already includes:

- `provider_session_id`
- `provider`
- `model`
- `transport_type`
- `process_key`
- `status`

So today there is a mismatch:

```text
refactor docs / new runner store
  -> expect workflow_provider_* tables

actual checked-in SQL migrations
  -> only show workflow_run_sessions
```

## 15. Practical Meaning Of The Gap

This means two things must be treated separately.

### A. Design intent

The intended new design is:

- FlowPilot persists provider-session metadata in
  `workflow_provider_sessions`
- FlowPilot persists normalized provider event history in
  `workflow_provider_events`
- approvals/questions also get dedicated provider tables

### B. Current repo reality

The current repository does not yet prove that this schema exists in Supabase.

Therefore:

- the SQL migration for the new provider tables is missing from this repo, or
- the migration exists elsewhere but is not checked in here, or
- the code path has not been fully cut over yet

## 16. Current Risk / Unknown

Because the repo is missing the checked-in migration, we cannot assume the new
Supabase-backed app-server persistence path works in a real environment.

Current risk statement:

```text
The new code references workflow_provider_sessions and related tables,
but this repo does not contain the SQL migration that creates them.
So if the live runner executes that path against a database without those
tables, writes will fail.
```

Operationally, that means:

- documentation target = new `workflow_provider_*` schema
- checked-in database schema = still shows old `workflow_run_sessions`
- live status = not proven from repo alone

## 17. Conservative Conclusion

At the moment, the safe conclusion is:

- yes, the refactor expects dedicated provider tables
- yes, this repo appears to miss the SQL migration for them
- no, we should not assume the current code path is fully working until the
  schema migration exists and the live path is verified

## 18. Phase 1 Audit Against R3-Migrate-Web-To-Desktop

Audit date: 2026-06-14

This audit checks the current repo state against
`requirements/10-Refactor/Note/R3-Migrate-Web-To-Desktop.md`.

Current result:

- done: 4
- partial: 6
- missing: 3

### Milestone Status

- `P1.0 - Inventory and contracts`: missing
- `P1.1 - Shared client core scaffold`: partial
- `P1.2 - Desktop shell, routing, bootstrap, and login`: partial
- `P1.3 - Project migration`: partial
- `P1.4 - Workflow and workflow step migration`: partial
- `P1.5 - Team migration`: done
- `P1.6 - Artifact migration`: partial
- `P1.7 - AI provider migration`: partial
- `P1.8 - MCP and Google Drive migration`: partial
- `P1.9 - Runner status migration`: partial
- `P1.10 - verification and cutover readiness`: missing

### Route Matrix Status

- `/login`: done
- `/settings/supabase` and `/setup/supabase`: done
- `/projects` and project detail/bindings/settings: partial
- `/workflows` and detail: partial
- `/workflow-steps` and detail: partial
- `/teams`: done
- `/artifacts` tabs: partial
- `/settings/ai-providers`: partial
- `/settings/google-drive-setup`: missing
- Jira MCP routes: partial
- `/settings/runner`: partial

### Main Reasons It Is Not Phase 1 Complete Yet

- There is no checked-in `P1.0` route inventory / acceptance checklist artifact.
- There is no checked-in `P1.10` parity checklist / verification package.
- Several desktop settings screens exist, but some are still simplified parity
  approximations rather than full route-equivalent behavior.
- Google Drive setup does not yet exist as a dedicated screen/flow.
- Runner status modal/header parity is not yet implemented.
- Project binding acceptance rules are not fully satisfied.
- Shared client core exists, but Admin Web does not appear to be consuming it
  yet, so gradual migration proof is still weak.

### Highest Priority Gaps To Close Next

1. Add a checked-in Phase 1 checklist artifact for `P1.0` and `P1.10`.
2. Complete project binding acceptance:
   - require at least one binding on create
   - reject duplicate binding paths
   - cover fallback behavior in tests
3. Add dedicated runner health status in desktop header with modal parity.
4. Move provider/model mapping logic out of desktop UI and into shared core.
5. Split generic MCP screen into route-equivalent Google Drive and Jira setup
   behavior, or explicitly narrow Phase 1 scope if that is intentional.
