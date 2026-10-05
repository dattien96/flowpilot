# CA-1206 — Supervised runners never idle-exit (live stack drains)

## Evidence
Ledger fingerprint `lifecycle|supervised-idle-drain` (medium): the runner
running under `FLOWPILOT_LIFECYCLE_MODE=supervised` entered `idle_grace`
~30s after boot with zero leases and drained to shutdown whenever no
desktop window was attached — recurring full-stack teardowns mid-watching.

## Root cause
`transitionOnceLocked` treated only `ModePersistent` as never-idle-exit.
`ModeSupervised` fell through to the client-managed edges: zero leases +
zero work → `idle_grace` → `draining_shutdown`. That contradicts the CP-81
Task-419 contract documented at the spawn site (`scripts/supervisor.js`):
"supervised mode — the runner exposes the lifecycle API but never
idle-exits; shutdown/restart authority is coordinated via fenced
supervisor.cmd records". The types.go comment had it backwards.

## Fix
`transitionOnceLocked`: the single `persistent` predicate becomes
`neverIdleExit = ModePersistent || ModeSupervised`, covering all three
idle edges (boot grace with no leases, ready with no leases/work,
orphaned-work drained). Supervised runners still take fenced
`/system/shutdown` and `/system/restart` commands, still negotiate
confirmations, still fence by instance ID + generation — only self-initiated
idle drain is gone. types.go mode docs corrected to match.

## Scope
Client-managed semantics untouched (all lifecycle tests run
ModeClientManaged). The vibe_watch.py TUI-lease mitigation stays as
belt-and-suspenders for older runners.

## Tests
`TestLifecycle_SupervisedNeverIdleExits` — supervised boot with zero leases
settles ready (never idle_grace); last-lease release + idle deadline passes
in ready. Existing `TestLifecycle_PersistentModeNeverIdleExits` +
client-managed drain tests unchanged and green.
