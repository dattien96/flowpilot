# BUG-506 — Writer spawn reads `workflow_steps` from Supabase with no embedded-pack fallback; a transient catalog outage dead-ends a live flow

## Status
FIXED — unit-verified, 2026-09-26, fixed build.

- Fix (CA-1008): `spawnChildRun` now passes the parent's canonical
  `chatFlowRef` as the internal `FlowRefFallback` (never mode-validated —
  not a user-declared mount). When `createRun`'s step synthesis fails to
  resolve the mirror-row `WorkflowID` during a catalog outage, it retries
  `flowStepsFromDefinition` against the fallback, which resolves from the
  embedded pack with no store. Fail-closed semantics preserved when
  neither resolves.
- Unit: `bug506_spawn_catalog_outage_test.go` (3 tests, green): child
  survives outage via canonical ref; no ref still fails closed; spawned
  child inherits the parent flow ref for its own descendants.

## Live-found during
`run-15708` (bug-harness), 2026-09-26, build 8c95a5bb.

- Flow had already spawned contract-planner (00:40), context hop (01:01) and
  reproducer (01:09) fine — catalog was reachable.
- At 01:11 `implement` spawn attempted `GET …/rest/v1/workflow_steps?…`
  (Supabase) which timed out (verified independently: same endpoint took
  7.7–20s or timed out entirely from the same machine).
- `flow_advance_writer_spawn_failed` → `catalog_unavailable` → escalate
  parked the loop. Two operator `continue`s re-attempted the spawn and hit
  the same timeout (~70s each); the second resume logged
  `vibe_requirement_resume_no_advance` ("requirement resume produced no
  dispatch — loop may rely on stall watchdog").
- A third manual retry eventually landed when the network blip passed.

## Problem

- Spawn-time catalog read goes to Supabase even for **built-in pack flows**
  whose full topology exists in the embedded pack — no local mirror/fallback
  read path is consulted at spawn time (the embedded-definition fallback
  exists only at run *create* time, BUG-426).
- `blockReason: "requirement"` on a spawn failure means `continue` does not
  reliably re-drive the spawn (`vibe_requirement_resume_no_advance`).
- A ~60–90s Supabase hiccup is enough to strand a flow mid-chain; recovery
  required manual retries with no card telling the operator "transient
  catalog fetch — retry".

## Suggested fix directions

- For built-in pack flows, resolve `workflow_steps` from the embedded pack
  definition when the Supabase read fails (the mirror is a cache of the same
  data).
- Retry with backoff inside the spawn path before escalating (bounded, e.g.
  3 attempts), and make the escalate reason carry `transient_catalog` so a
  resume knows to re-drive the spawn rather than park silently.
- `requirement`-blocked resumes should produce a dispatch for spawn-failure
  parks, or the blockReason should be `escalate` so continue works as
  expected.

## Evidence
- log `flow_advance_writer_spawn_failed` ×2 on run-15708 with
  `catalog_unavailable: Get https://ipgxvrxrhfhaeskfvctr.supabase.co/…`
- manual curl to the same Supabase host: attempt1/2 timeout(20s), attempt3
  401 in 10.4s — endpoint alive but intermittently very slow.
