# Task-103: Engine Local Store And Drive Sync

## Metadata

- Document ID: `Task-103`
- Title: `Engine Local Store And Drive Sync`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `None`
- Related Documents: [Task-096: Commit-History Ledger](./Task-096-Commit-History-Ledger.md), [Task-097: Feature Catalog And Resolver](./Task-097-Feature-Catalog-And-Resolver.md)
- Replaces: `None`
- Tags: `contextsync, storage, google-drive, local-runner`

## AI Quick View

### Summary

- Persist engine data under `<target>/.flowpilot/` and sync the shared, machine-independent parts to the project's Drive folder.
- Reuses the existing chat-sync mechanism (`chat_session_sync.go`) under a `context-engine/` prefix — same NDJSON `_index` + manifest + SHA256.
- Machine-specific/ephemeral data (tooling status, baselines, indexes) stays local; git remains the source of truth for raw history.

### Current Ask

- Implement `internal/contextsync/` per CP-35 §4.8 and `SD-17 §5.1`.

### Key Decisions

- `T-1` Sync only `feature_history.ndjson`, `features.ndjson`, `flow-rules.json` to `context-engine/`.
- `T-2` Reuse `ensureGoogleDriveFolderPath` / `upsertGoogleDriveFile` and the chat-sync index/manifest pattern; reuse the `CP-33` Drive folder.

### Constraints

- Depends on Task-096 / Task-097 (data to sync). Must not modify chat-sync behavior — reuse only. Machine-specific data must never sync.

### Open Questions

- Sync trigger debounce interval.

### Source Refs

- `CP-35 §4.8` (P-8); `SD-17 §5.1`, §9; `SS-14 AC-15`.

## 1. Goal

Engine data is durable locally and the shared parts round-trip through the project's Drive folder using the same mechanism as chat.

## 2. Parent Links

- coding plan: `CP-35` P-8
- tech design: `SD-17` §5.1, §9
- system spec: `SS-14` AC-15
- specific upstream ids: `P-8`, `AC-15`

## 3. Trigger

The user needs engine data both local and synced (chat already syncs); this slice closes that for the engine without new infra.

## 4. Exact Change

- `T-1` `internal/contextsync/` — write engine files under `.flowpilot/` per `SD-17 §5.1`.
- `T-2` `syncContextEngine(projectID)` — build manifest (SHA256) for the three shared files; upload to `context-engine/` via reused helpers; write `context-engine/_index/manifest.json`.
- `T-3` keep machine-specific/ephemeral data local-only; trigger sync on ledger/catalog rebuild + step complete (debounced).
- `T-4` unit tests: manifest + SHA256, sync-vs-local split.

## 5. Touched Areas

- files: `apps/local-runner/internal/contextsync/*` (reuses `chat_session_sync.go` helpers)
- modules: `contextsync`
- routes: none
- tables: none

## 6. Acceptance Check

- CP-35 P-8 DoD: shared files appear under `context-engine/` in the project Drive folder with a manifest; tooling/baseline never sync.
- `go test ./internal/contextsync/...` passes.

## 7. Out of Scope

- Changing chat sync (reuse only); syncing machine-specific data; the Supabase mirror (CP-35 §6, optional).

## 8. Completion Notes

- result: planned
- follow-ups: optional Supabase mirror per CP-35 §6
- upstream docs updated: none
