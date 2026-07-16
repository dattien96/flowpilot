# Task-258: Per-Project Dispatch Log And Drive Sync

## Metadata

- Document ID: `Task-258`
- Title: `Per-Project Dispatch Log And Drive Sync`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Codex review`
- Created: `2026-07-17`
- Last Updated: `2026-07-17`
- Parent Documents: [CP-51: Durable Turn Dispatch State Machine And Recovery Reconciliation](../../07-Coding-Plan/todo/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [SD-24: Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md)
- Child Documents: `None`
- Related Documents: [Task-248](./Task-248-Durable-Dispatch-Record-And-State-Machine-Core.md) (store API), [Task-253](./Task-253-Supabase-Runtime-Versioned-And-Fail-Closed.md) (session_runtime fail-closed — still local/Supabase session blob, not dispatch authority), chat session Drive sync (`chat_session_sync.go`)
- Replaces: Supabase-as-default-authority for CP-51 dispatch tables/RPC client path (local NDJSON + Drive becomes product path)
- Tags: `agent-flow-engine, durable-turn, dispatch-store, google-drive, multi-project, persistence`

## AI Quick View

### Summary

- CP-51 initially sketched **two backends** for the durable turn store: local single-writer `dispatch.ndjson` **and** Supabase tables + transactional RPCs (`dispatch_records`, `dispatch_run_stop_state`, `run_protocol_activations`, `dispatch_effects`, `repair_records`, …).
- Product decision (2026-07-17): **do not use Supabase as the default transport for dispatch state.** Authority is **local per-project NDJSON**, portability is **Google Drive chat-sync** (same pipeline as sessions), matching how FlowPilot already moves chat artifacts across machines.
- Layout: `.flowpilot/chats/<project_id>/dispatch.ndjson` (+ `dispatch.lock`). Drive: `chat-sessions/dispatch/dispatch.ndjson` under the project sync root.
- Runner no longer opens a Supabase dispatch client. SQL migrations that created tables are **dropped** by a follow-up migration so environments that applied the earlier turn are cleaned.

### Current Ask

- Document and close the move: per-project local shard, Drive upload/download hooks, remove Supabase dispatch store code, drop cloud tables. Keep `DispatchStore` API and in-memory tests unchanged for unit contracts.

### Key Decisions

- `T-1` **One log per project**, not one global log for all projects: path `chats/<project_id>/dispatch.ndjson`. `DispatchRecord.ProjectID` is required at `CreatePrepared`.
- `T-2` **Drive sync reuses chat-sync**, not a new cloud product: upload on `syncChatRunToDrive`; download **before** session index restore on `restoreChatRunTreeFromDrive` so recovery sees durable states first.
- `T-3` **Supabase dispatch tables/RPC are not the product path.** Optional historical migrations are reversed by `DROP` migration. Session runtime fail-closed (Task-253) remains a separate concern (session blob), not dispatch authority.
- `T-4` Multi-project hub (`multiProjectDispatchStore`) routes by `project_id` / `run_id→project` index; process lock remains per project shard.

### Constraints

- Must not break existing chat-session Drive sync contracts (manifest, index, child runs).
- Dispatch upload/restore is **best-effort** relative to session sync: session success is not rolled back if dispatch upload fails (logged); empty remote dispatch is OK for projects synced before this feature.
- Two machines mid-flight on the same project remain a product conflict class (same as session sync): prefer single-writer usage; no automatic multi-writer merge of two divergent logs.

### Open Questions

- `Q-1` Per-run shard (`dispatch/<run_id>.ndjson`) vs whole-project log — **resolved for v1 as whole-project log** for simpler Drive path; may split later if files grow large.
- `Q-2` Whether Drive sync should be fail-closed when V2 is enabled and upload fails — default best-effort; revisit if operators need hard guarantees.

### Source Refs

- Code: `dispatch_store_open.go` (multi-project hub), `dispatch_store_local.go`, `dispatch_drive_sync.go`, `chat_session_sync.go` (upload/restore hooks), `dispatch_record.go` (`ProjectID`), `cli/root.go` (`OpenDispatchStoreForServe`).
- Drop migration: `supabase/migrations/20260717140000_drop_cp51_dispatch_supabase_tables.sql`.
- Prior (superseded as default): `20260717120000_dispatch_records_cp51.sql`, `20260717130000_dispatch_records_cp51_rls.sql`.

## 1. Goal

Make durable turn dispatch **multi-project local** and **portable via Google Drive chat sync**, and **retire Supabase tables/RPC as the default CP-51 dispatch backend**, so a second machine restoring a project chat also restores dispatch authority for recovery.

## 2. Parent Links

- coding plan: CP-51 (persistence transport decision / post-foundation hardening)
- tech design: SD-24 (§6 store contract; local commit log remains; Supabase §6.5 becomes non-default optional)
- system spec: SS-17 (uncertain/repair still surface; transport of records is local+Drive)
- specific upstream ids: CP-51 INV-1/INV-2 recovery portability; multi-project isolation

## 3. Trigger

1. A single global `chats/dispatch.ndjson` cannot cleanly scope multi-project isolation or per-project Drive folders.
2. Supabase dispatch RPC was only a **subset** of `DispatchStore` and diverged from how chat already moves across machines (Drive).
3. User product preference: **same Drive path as existing chat sync files**, not a second cloud authority.

## 4. Exact Change

### 4.0 What moved off Supabase (capture)

| Was (CP-51 early sketch) | Now (Task-258) |
|--------------------------|----------------|
| `dispatch_records` table + RPC CAS | Local NDJSON lines in per-project log |
| `dispatch_run_stop_state` | Same fields in local log projection / lines |
| `run_protocol_activations` | Activation lines in local log (non-prunable in projection) |
| `dispatch_effects` / release / repair rows | Local log effects / repair records |
| `dispatch_*` PostgREST RPC client in runner | **Removed** (`dispatch_store_supabase*.go` deleted) |
| Open store via `FLOWPILOT_*_SUPABASE_DSN` | `OpenDispatchStoreForServe(chatsRoot)` → multi-project local only |
| Cloud multi-machine = shared Postgres | Cloud multi-machine = **Drive file** `chat-sessions/dispatch/dispatch.ndjson` |

**Not moved / still separate:**

- Task-253 **session_runtime** versioning/fail-closed on Supabase **session** blob (workflow provider session) — not the dispatch store.
- FlowPilot catalog/workflow tables on Supabase unchanged.

### 4.1 Local layout

```text
<runner_workspace>/.flowpilot/chats/
  sessions.ndjson                    # multi-project sessions (existing)
  <project_id>/
    dispatch.ndjson                  # NEW shard
    dispatch.lock
```

### 4.2 Drive layout (per project sync root)

```text
<Drive project chat-sync root>/
  chat-sessions/
    _index/sessions.ndjson           # existing
    … manifests / provider files …
    dispatch/
      dispatch.ndjson                # NEW — whole project dispatch log
```

### 4.3 Code

- `T-1` `multiProjectDispatchStore` + `ProjectID` on `CreatePrepared`.
- `T-2` `syncDispatchLogToDrive` on successful chat sync upload path.
- `T-3` `restoreDispatchLogFromDrive` **before** remote session index load on restore.
- `T-4` Delete Supabase dispatch Go clients; comment migrations as non-default; add **DROP** migration for tables/functions/indexes created earlier.

### 4.4 Drop migration (apply on Supabase if 120/130 were applied)

See `supabase/migrations/20260717140000_drop_cp51_dispatch_supabase_tables.sql`:

- Drop RPCs: `dispatch_create_prepared`, `dispatch_cas_advance`, `dispatch_request_run_stop`, `dispatch_get_run_protocol_version`.
- Drop tables: `dispatch_audit`, `dispatch_intent_clears`, `repair_records`, `dispatch_effects`, `dispatch_records`, `dispatch_run_stop_state`, `run_protocol_activations` (order respects FKs if any).

## 5. Touched Areas

- files: `dispatch_store_open.go`, `dispatch_drive_sync.go`, `dispatch_record.go`, `dispatch_live.go`, `chat_session_sync.go`, `cli/root.go`, tests; deleted `dispatch_store_supabase.go`, `dispatch_store_supabase_rpc.go`; migrations `20260717120000_*` (note), `20260717130000_*` (note), **`20260717140000_drop_*` (new)**.
- modules: `internal/runner` persistence + chat sync.
- routes: none new (reuses chat sync / restore).
- tables: **drop** CP-51 dispatch Supabase objects.

## 6. Acceptance Check

- `V-1` CreatePrepared with `project_id=A` writes only under `chats/A/dispatch.ndjson` (not a global log).
- `V-2` Export/Import project log round-trip restores `Get` state (unit: `TestCrashMatrix_MultiProjectShardAndExportImport`).
- `V-3` Chat sync upload path calls dispatch upload; restore path imports dispatch before session resume (code hooks present).
- `V-4` Runner has no `NewSupabaseDispatchStore*` / DSN open path for dispatch.
- `V-5` DROP migration is present and drops all objects created by 120 (+ RLS 130 is no-op after drop).

## 7. Out of Scope

- Full CP-51 §10 crash-matrix finish line (Task-255 phase 2 remains).
- Automatic multi-writer merge of two divergent project logs.
- Re-implementing full DispatchStore as Supabase RPCs.

## 8. Completion Notes

- result: **done (2026-07-17)** — per-project local shard + Drive sync hooks landed; Supabase dispatch clients removed; DROP migration added for previously created tables.
- follow-ups: optional fail-closed upload when `FLOWPILOT_DISPATCH_V2=1`; SD-24 wording pass to mark Supabase §6.5 non-default.
- upstream docs updated: CP-51 Child Documents + this task; migrations annotated.
- DoD checklist:
  - [x] V-1 per-project path
  - [x] V-2 export/import test
  - [x] V-3 Drive hooks on sync/restore
  - [x] V-4 no Supabase dispatch client
  - [x] V-5 DROP migration file
