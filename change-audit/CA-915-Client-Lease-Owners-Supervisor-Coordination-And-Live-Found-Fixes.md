---
id: CA-915
title: TUI/Desktop lease owners, supervisor coordination, cross-client E2E, live-found fixes
type: Feature
feature: runner-lifecycle
date: 2026-09-22
status: done
---

## Context

CP-81 (SS-24/SD-28) completion: the runner owns its lifetime via
generation-scoped leases, heartbeat TTL, 30s idle grace, fenced two-phase
shutdown/restart, and durable stop-all. CA-914 landed the manager, HTTP
surface, and runnerboot decoupling. This note covers Task-417 (TUI lease +
close UX + reconnect), Task-418 (Desktop lease owner + system UX),
Task-419 (supervisor coordinated restart/shutdown), Task-420 (cross-client
E2E + live validation), and the real bugs the live runs surfaced — including
two CP-71 durability defects found only by exercising a real runner.

## Change

- `internal/tui/app/lifecycle.go` (new): lease registration on connect,
  heartbeat loop at the server-provided interval, reconnect grace on
  `runner_died`/`planned_restart`, close-intent decision (sole client →
  plain quit; shared/protected work → close dialog; unreachable → local
  quit), lease release on quit, and fenced two-phase shutdown for the
  explicit "Turn off FlowPilot" path (`cmdShutdownAndQuit` still POSTs
  /system/shutdown → pinned telemetry tests unchanged).
- `internal/tui/app/app.go` + `model.go` + `root.go`: quit sites route
  through exit-intent; Ctrl+C/Ctrl+D stay synchronous `QuitMsg` when no
  dialog is needed (pinned tests preserved); lifecycle status chip on the
  status line; headless mode registers a lease too.
- `apps/desktop-flowpilot/src/lifecycle/desktopLifecycle.ts` (new, pure TS):
  idempotent register/heartbeat/release owner, before-quit flow (sole
  client → release; shared/busy → confirm dialog; dead runner → quit),
  `runner_died` → notification + renderer close, `planned_restart` →
  reconnect grace. Ports (transport/dialog/notify/quit) injected for tests.
- `electron/lifecycle.ts` + `main.ts` + `preload.ts`: thin adapter wiring
  real `dialog`/`Notification`/`app` into the pure owner; the unconditional
  before-quit shutdown POST is removed — quit releases only this client's
  lease; bridge exposes lifecycle status/system actions to the renderer.
- `HttpWsRunnerClient`/`MockRunnerClient`/`contract.ts`: lifecycle DTOs +
  `restartStack`/`shutdownStack` two-phase calls through the bridge;
  `RunnerHealth` gains additive lifecycle fields; store +
  `RunnerStatusIndicator` surface lifecycle status.
- `scripts/supervisor.js`: `startRunnerProcess` launches supervised mode
  (`FLOWPILOT_LIFECYCLE_MODE=supervised` + runnerInstanceId env); fenced
  supervisor-command validation — stale `runnerInstanceId`/`restartId`
  records can never kill a newer generation; planned-exit restarts only the
  runner that requested it; unexpected exits follow restart policy; adopt
  path caches the adopted instance and fails closed on malformed records;
  `require.main` gate + injected spawn/kill seams for tests.
- `internal/runner/cp81_lifecycle_e2e_test.go` (new): 18-test real-HTTP
  harness (live sweeper, short TTLs) covering the whole Task-420 matrix —
  shared TUI/Desktop attach, single-client close, last-client idle shutdown,
  crash TTL expiry, idle-grace reattach, active-work fencing, forced
  shutdown stop-all, planned-restart reconnect, reconnect timeout,
  unplanned death, idle-only stale replacement, busy-stale update-pending,
  legacy handling, workspace mismatch, foreign-port protection, stale
  supervisor command fencing, MCP-child shutdown protection, no
  go-run/compiled-runner orphans.

## Live-found bugs fixed (real runner + Devin SWE-2)

- **Boot-lock torn-write race** (`runnerboot/boot_lock.go`): the lock file
  was created empty via `O_EXCL` then PID-written in a second step; a
  contender reading mid-write saw malformed content, classified it stale,
  reclaimed it → two starters both spawned onto one port
  (`TestEnsureRunner_ConcurrentStartsSingleWinner` flake, reproduced ~1/5).
  Fixed with atomic claim (unique temp file + `os.Link`, `O_EXCL` fallback +
  write-grace in stale detection). 20/20 iterations + `-race` green.
- **Worktree binding erased by later session writes** (CP-71 durability,
  found by live restart test): `sessionStateOf` never stamped
  `rs.worktree` fields, and `sessions.ndjson` is last-write-wins per run —
  every post-create write (running/completed/cancelled) erased the binding,
  so boot GC pruned the "orphan" worktree containing unmerged user work
  with no `lost` notice. Fixed by stamping `worktreeFieldsToSession` into
  every snapshot. Reproduce-first:
  `TestE2EWorktree_PostBindingSessionWriteKeepsBindingHTTP` (red → green).
  Verified live: idle/running/completed rows now all carry the binding;
  post-restart boot GC left the bound worktree intact.
- **Resolve-after-restart self-deadlock** (`run_worktree_merge.go`
  `resolveWorktree`): the handler held `s.mu` while calling
  `loadPersistedRun` → `reconstructRunInternal`, which re-acquires `s.mu`
  — worktree resolve for any non-resident run blocked forever (pre-existing;
  GitNexus index predates these symbols — single caller is the HTTP
  handler). Fixed by releasing `s.mu` around the rebuild and re-checking
  under the lock. Verified live: POST …/worktree/resolve after a real
  runner restart completed in 141ms with `applied:true`.

## Tests

- `internal/tui/app/*lifecycle*_test.go`: all 14 Task-417 signatures —
  register on connect, heartbeat interval, release on quit, shared-runner
  close dialog, sole-client direct quit, unreachable runner local quit,
  reconnect on planned restart, reconnect timeout closes, runner-died
  notice, cancel keeps session, fenced shutdown path, headless lease,
  status chip.
- `apps/desktop-flowpilot/src/lifecycle/desktopLifecycle.test.ts`: all 11
  Task-418 signatures via injected ports — idempotent register, heartbeat,
  release on quit, shared/busy confirm dialog, dead-runner quit, died
  notification, planned-restart reconnect, lease fencing. Run through the
  phase-1 tsconfig compile + `node --require scripts/phase1-runtime.js`.
- `tests/phase1/supervisorLifecycle.test.ts`: all 9 Task-419 signatures —
  supervised env, fenced command validation (stale instanceId/restartId
  rejected), planned-runner-only restart, unexpected-exit policy, adopt
  caching, malformed record fail-closed.
- `internal/runner/cp81_lifecycle_e2e_test.go`: 18 real-HTTP E2E (above).
- `internal/runner/cp71_worktree_e2e_test.go`: +
  `TestE2EWorktree_PostBindingSessionWriteKeepsBindingHTTP`; full
  `TestE2EWorktree_*` suite green.

## Live validation (real runner, real Devin SWE-2 — no other providers)

- Two clients (tui + desktop kinds) registered against one generation;
  shared snapshot; heartbeat renewal; per-lease fencing (stale generation /
  bad token → typed 404/409); unauthenticated shutdown on an empty runner →
  202 → `ready→idle_grace→draining_shutdown→stop-all→stopped→exit` with
  requester attribution in the supervisor record.
- Real Devin run (`providerKey=devin`, `devin/swe-2-high`): turn in flight
  counted as protected work; busy shutdown → 409
  `lifecycle_confirmation_required` + confirmToken bound to
  inventoryRevision; confirmed replay → 202 + `restartId` + deadline →
  `draining_restart` → fenced supervisor command written with
  `runnerInstanceId` + `restartId`.
- CP-71: worktree bound per chat (`cht_*`), Devin wrote `CP71_LIVE2.txt`
  only inside the worktree; runner restart left binding + worktree intact
  (pre-fix they were GC'd); resolve `apply_patch` merged into the main tree
  and cleaned the worktree.

## Risk

- `internal/runner` full suite keeps the known environment-dependent
  baseline (provider detection counts, Drive/Firebase/Supabase fixtures,
  flow-engine flakes, 2 TUI baseline failures) — verified identical on
  clean HEAD via detached worktree; zero lifecycle-related failures in the
  filtered set.
- Desktop `npm run typecheck`/`build` blocked by pre-existing
  `store.chat-mode-persist.test.ts` TS2554 (present at HEAD; test not
  edited per additive-tests-only). Focused lifecycle tests pass via the
  phase-1 compile path.
- Minor attribution gap noted live: supervisor requester shows the runner's
  own pid for curl peers (logged, non-blocking).
- SIGTERM on a busy shared runner is warn-then-force (double signal within
  5s) — verified live; clients must use the fenced API for graceful global
  shutdown.
