# BUG-674 — spawn_agent Consults the Relay Leg's Phantom Loop

## Metadata

- Document ID: `BUG-674`
- Title: `spawn_agent admission resolves loop state on the addressed (relay) leg — "loop is stopped" while the launcher loop is alive`
- Phase: `bugfix`
- Status: `FIXED — CA-1238`
- Owner: `flowpilot-runner`
- Created: `2025-01-24`
- Parent Documents: `BUG-669` (relay-leg discriminator), `BUG-432`, CP-36
- Tags: `spawn-agent, relay-leg, cohort, severity:high`

## Evidence (live run-752742)

- Relay leg `run-3` answered `parent run "run-3" loop is stopped — no
  further spawns` while the launcher leg's sprint loop was still running;
  the cohort spawn for `spec_align`/`reviewer` could not fire until a fresh
  leg was minted.

## Root Cause

- `spawnChildRun`'s admission guard called `loopStateFor(parentRunID)` —
  the addressed leg. A relay leg carries `flowEngineDriven` but no topology,
  so its (phantom or stopped) loop was consulted instead of the real flow
  owner's.

## Fix

- The blocked/sealed spawn guards now resolve
  `flowOwnerLoopStateForLocked(parentRun, parentRunID)` — the BUG-669
  owner resolver — for status, block reason, diagnostics, and error text.
  Genuine launcher/root runs are unaffected (resolver is identity there).

## Regression Test

- `bug674_spawn_relay_owner_loop_test.go` — red: `loop is stopped` refusal;
  green: spawn admitted via the launcher leg's running loop.
