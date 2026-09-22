---
id: CA-914
title: Runner-owned shared lifecycle authority + lifecycle-aware runnerboot
type: Feature
feature: runner-lifecycle
date: 2026-09-22
status: done
---

## Context

CP-81 (SS-24/SD-28) moves runner lifetime from client-owned to runner-owned:
TUI and Desktop share one runner via generation-scoped leases, heartbeat+TTL,
a 30s idle grace, two-phase fenced shutdown/restart, and durable stop-all —
while TUI/Desktop process death can no longer kill a shared runner, and
stale builds are replaced only when provably idle.

Covers Task-414 (lease registry + state machine), Task-415 (HTTP surface +
durable stop-all), Task-416 (runnerboot decoupling + stale replacement).

## Change

- `internal/lifecycle/` (new package): `Manager` owns phases
  (starting→ready→idle_grace/orphaned_work→draining_*→stopped),
  generation-scoped leases with hashed tokens, heartbeat TTL expiry, fixed
  -point state evaluation, single-use confirmation tokens bound to
  inventoryRevision, restart reconnect metadata. Injectable clock/work
  snapshot/phase callback; no os.Exit, no process spawn, no HTTP.
- `internal/runner/lifecycle_api.go`: `/system/lifecycle` + client
  register/heartbeat/release routes; `HandleSystemActionRequest` implements
  the D-4 two-phase contract (202 accept / 409 + confirmToken) for
  `/system/shutdown` + `/system/restart`. `/health` gains additive
  runnerInstanceId/generation/protocolVersion/buildId/lifecycleMode/phase.
- `internal/runner/lifecycle_work.go`: `LiveWorkSnapshot` enumerates
  turns/post-turn gates/inline flows/agent loops/scaffolds provider-
  agnostically; `StopAllForSystemAction` reuses `stopAgentLoop` durable
  fencing per root + scaffold cancels; `drainingErr` gates new work once
  draining.
- `internal/cli/root.go`: serve boots `lifecycle.NewManager` (mode via
  --lifecycle-mode / FLOWPILOT_LIFECYCLE_MODE, default persistent), attaches
  it to Runner + InteractiveService, keeps `describeRequester(r)` inside the
  literal shutdown/restart handlers (CA-911/913 contract), dedupes drains via
  `sync.Once`, and `runSystemDrain` does stop-all → fenced supervisor command
  (runnerInstanceId + restartId + requester) → bounded cleanup → exit.
  SIGINT/SIGTERM on managed runners is warn-then-force through the manager.
- `internal/lifecycle/buildid.go` + `runnerboot/build_identity.go`:
  vcs.revision / exe-SHA-256 build identity shared by both sides of the wire,
  so `go run` temp binaries are distinguishable.
- `runnerboot`: classification enum (compatible/idle_stale/busy_stale/
  protocol_incompatible/legacy_unknown/workspace_mismatch/port_conflict),
  idle-only fenced stale replacement with wait-for-instance-gone +
  KillRunnerFenced fallback, machine boot lock with stale recovery, detached
  shared spawn (`Setsid` unix / `DETACHED_PROCESS`+no job object windows,
  `--lifecycle-mode client-managed`), and foreign-listener port_conflict
  detection — unknown processes are never killed.
- `internal/tui/client/lifecycle.go` (new): lease/lifecycle DTOs + register,
  heartbeat, release, fenced system actions with typed 409 decode.

## Tests

- `internal/lifecycle/manager_test.go`: 20 required signatures + matrix —
  register/heartbeat/release fencing, idempotency, TTL expiry → idle grace,
  reattach cancels shutdown, orphaned-work transitions, two-phase confirm
  tokens, concurrent serialization, tokens-hashed invariant.
- `internal/runner/lifecycle_api_test.go`: HTTP surface — route registration,
  lease lifecycle over HTTP, typed error status mapping, 409 confirm-token
  flow, busy/idle shutdown decisions, work inventory contents, stop-all
  reuse of stopAgentLoop, drain gate rejection.
- `internal/tui/runnerboot/runnerboot_cp81_test.go`: all 13 §7 signatures —
  deterministic build identity, compatible reuse, idle-stale auto-replace
  (fenced by expectedInstanceId), busy-stale no-replace, protocol/workspace
  rejection, legacy-unknown no-auto-replace, foreign-port never killed,
  concurrent single-winner boot, client-managed spawn args, detached spawn
  (no job object), fenced kill requires instanceId, bounded stale-lock
  recovery.

## Risk

- Full `internal/runner` suite carries ~27 pre-existing environment-dependent
  failures (provider detection counts, Drive/Firebase/Supabase fixtures,
  flow-engine flakes) — verified identical on clean HEAD via a detached
  worktree; none introduced by this change.
- `cmdShutdownAndQuit` intentionally still POSTs /system/shutdown (explicit
  "stop runner" UX); the normal-quit lease release lands with Task-417.
- Windows job object + CA-474 attached spawn remain available for explicit
  stack shutdown; only the shared spawn path detaches.
