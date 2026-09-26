# BUG-510 — Steps endpoint 404s after restart though durable step rows exist

## Status
FIXED — 2026-09-26 (CA-1011). Same bug class as BUG-508: read endpoint
consulted only the in-memory `s.runs` cache instead of reading through to
the durable store.

## Found during
Deep review B-6 (`requirements/07-Coding-Plan/note/
CP-Deep-Review-12CP-2026-09-26.md`), verified against source.

## Observed

`GET /client/workflow-runs/{id}/steps` consulted `s.runs[runID]` and
returned 404 on a cache miss — while `ListWorkflowSteps` (durable) still
held the run's step rows. After a restart, a run's steps were
unrecoverable via the API even though the data was on disk — the same
asymmetry BUG-508 fixed for the snapshot endpoint.

## Root cause

The steps handler short-circuited on `s.runs` membership; the durable
fallback helper added for BUG-508 (`durableRunSnapshot`) was not reused.

## Fix (CA-1011)

`handleRunSteps` falls back to `durableRunSnapshot(runID)` on an
in-memory miss — read-only projection, then serves the durable step list.
Genuinely unknown ids still 404; store errors surface 5xx, not a
misleading 404.

## Tests

`bug510_steps_runtime_rehydrate_test.go` — seeds durable session + step
rows with an empty `s.runs`, asserts 200 with the persisted step/status,
and that an unknown id still 404s. Green.

## Round 2 (2026-09-26, CA-1020) — envelope hydration

The first fix copied only `ProviderKey` from the durable session row;
`Model`/`YoloMode` on the response envelope stayed empty after restart.
Round 2 copies `sess.ModelName` and `sess.Yolo` too — step-level values
still come from the durable workflow-step rows.

## Tests (round 2)

- `TestBug510_StepsRuntimeHydratesModelAndYolo` — envelope model/yolo
  populated from the durable session; step-level model preserved;
  unknown run still 404s.
