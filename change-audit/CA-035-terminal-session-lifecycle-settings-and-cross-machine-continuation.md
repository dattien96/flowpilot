# CA-035: Terminal Session Lifecycle, Settings, and Cross-Machine Continuation

## Scope

This audit records the full CP-21 terminal-session change set, not just Task-02b. The work spans the admin-web settings and workflow UI, the workflow runtime, the Supabase data model, the local-runner session lifecycle, and the CP-21 plan note. The goal was to make terminal sessions durable, configurable, manually terminable, and recoverable across machine handoff.

## Completed

- Added a project-level session idle TTL setting in the admin UI and persisted it through Supabase so the runner can share the same lifecycle policy across machines.
- Added workflow-run UI support for manually terminating a live session before the idle timeout expires.
- Added the `session_idle_ttl_minutes` project field, payload mapping, and migration to persist the shared timeout configuration.
- Expanded the workflow runtime to read the TTL, create and reuse sessions by scope, persist provider session metadata, and keep portable checkpoints in `workflow_run_sessions.metadata_json.checkpoints`.
- Added same-machine reconnect handling for dead sessions and bootstrap replay handling for cross-machine handoff when the original local Codex store is unavailable.
- Added structured `session_dead` error handling across the local runner and HTTP gateway so retry paths can distinguish dead transport from other provider failures.
- Added idle-session tracking in the runner with `LastUsedAt`, a background sweeper, and configured TTL support.
- Updated the CP-21 checklist to mark the terminal-session delivery items complete.

## Verification

- Ran `go test ./internal/runner ./internal/cli` in `apps/local-runner`.
- Ran `npm test -- --run src/features/workflow-engine/workflow-start-runtime.test.ts src/data/repository/local-runner/http-local-runner-gateway.test.ts` in `apps/admin-web`.
- Confirmed the workflow-runtime tests cover dead-session reconnect, non-`session_dead` failures, and the cross-machine bootstrap replay path.
- Confirmed the runner tests cover provider-session resume seeding and idle-sweeper behavior.

## Residual Notes

- The persisted checkpoint format is prompt/output history stored in `workflow_run_sessions.metadata_json`; it does not yet include a separate rolling summary or artifact reference replay.
- Cross-machine recovery still depends on the bootstrap prompt being sufficient for the conversation length and complexity.
- The full unstaged change set has a broad blast radius across settings, workflow runtime, API routes, local-runner session lifecycle, and Supabase schema.
