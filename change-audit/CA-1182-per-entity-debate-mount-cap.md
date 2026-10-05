# CA-1182 — owner-debate mount cap accounted per gated entity

## Defect

`interactiveRun.vibeDebateMounts` was a single per-sprint counter
(BUG-595). Three owner-debate mounts resolving member A's gate violations
exhausted the whole sprint budget, so the FIRST gate block on an unrelated
member B hit the cap and escalated to a human park instead of remediating.
On a multi-member review cohort this turns one stubborn member's debate
loop into a sprint-wide remediation outage — the next live turn stalls on
an escalate card for a member that never got a single debate.

## Fix

- `vibeDebateMountsByEntity map[string]int` on `interactiveRun`, keyed by
  `vibeDebateEntityKeyLocked(gatedRunID)`: the gated child's flow-node
  `label` (stable across leg respawns — a replacement leg is still the
  same entity), `run:<id>` fallback when the child is unresolvable, and a
  shared `hub` bucket for un-gated (drift-only) diverts.
- `stashVibeFlowForDebate` increments both the sprint total and the entity
  bucket in the same atomic park.
- `startVibeOwnerDebate` caps when: the entity bucket reaches
  `maxVibeDebateMountsPerSprint` (3), OR the sprint total reaches the new
  `maxVibeDebateMountsSprintCeiling` (9) aggregate bound, OR a legacy
  resumed state has the int set with no map (pre-fix sessions keep the old
  whole-sprint cap).
- Sprint reset (`takeNextVibeSprintLocked`) and the pending-run normalize
  path clear both counters. The map is persisted through `SessionState` +
  `ndjsonSessionRecord` (new `vibe_debate_mounts_by_entity` field,
  `copyStringIntMap` round-trip) so a restart mid-sprint does not re-spend
  a fresh budget.

## Regression evidence

- `bug1182_per_entity_debate_cap_test.go`: (a) three mounts on entity A
  cap A's 4th but do NOT block entity B's first mount; (b) sprint take
  clears both counters; (c) sprint ceiling still bounds total mounts
  across distinct entities.
- BUG-595 mount-cap test passes unchanged (legacy fallback clause).
- Debate/vibe/sprint suite green.

## Files

- `apps/local-runner/internal/runner/vibe_debate.go` — ceiling const +
  `vibeDebateEntityKeyLocked`
- `apps/local-runner/internal/runner/vibe_gate.go` — per-entity cap check
- `apps/local-runner/internal/runner/vibe_cp.go` — ledger increment +
  sprint reset
- `apps/local-runner/internal/runner/interactive_service.go` — run field +
  snapshot
- `apps/local-runner/internal/runner/interactive_resume.go` — restore +
  pending-reset
- `apps/local-runner/internal/runner/workflow_store.go`,
  `local_file_session_store.go` — durable field + mappings +
  `copyStringIntMap`
