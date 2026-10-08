# BUG-633 — Audit settle during a straggler RUNNING step vetoes the sprint boundary then seals the whole run `done` with plan tasks remaining → premature `flow_run_complete`, no deferred re-check

- **ID:** BUG-633
- **Severity:** High — silently ends a multi-task vibe run mid-plan; only
  `POST /workflow-runs/{id}/resume` (operator reopen) recovers it via
  `maybeReparkVibeSprintBoundary`. Nothing self-heals.
- **Status:** FIXED — CA-1236 (2026-10-08): straggler veto now defers the boundary (audit stays PENDING, re-check deferred) instead of returning false so the run seals done
- **Found:** run-297984 (`vibe-adopt` CP-05, 6-task plan),
  2026-10-06T02:48 — after sprint-3 (Task-053) audit, the run emitted
  `flow_run_complete_begin/done` + `flow_control_done` with
  `vibeTaskIndex 3/6`. Nine seconds later `flow_validate_ran` fired for a
  `validate` node that had still been RUNNING at settle time, followed by
  `flow_advance_skipped_loop_blocked` ("skipping auto-advance because loop
  is not advancing") — the deferred advance arrived after the loop was
  already sealed.
- **Second instance:** run-306526 (`vibe-tasks` CP-04, 4-task plan),
  2026-10-06T03:30 — same signature after sprint-1 (Task-042) audit:
  `flow_run_complete_done` + `flow_control_done` at task 1/4. Recovered
  with the same path (`/resume` → `vibe_sprint_boundary` repark →
  `agent-loop/continue` mounts Task-043). Confirms the defect is
  systematic across flow types, not adopt-specific.
- **Third instance:** run-306526 again, 2026-10-06T04:03 — sprint-2
  (Task-043) audit → `flow_control_done` at task 2/4, same recovery.
  Not every boundary fires — CP-05 sprints 4→5→6 auto-advanced cleanly
  when evidence was complete at audit settle. The trigger is a
  remediation-heavy sprint closing with legs still in flight (both CP-04
  hits landed during coder/debate remediation tails).

## Symptom

Event order on run-297984:

```
02:48:24 flow_audit_draft_built        (sprint-3 audit settles)
02:48:24 flow_run_complete_begin/done  → loop_status=done
02:48:33 flow_validate_ran             (validate leg finishes late)
02:48:33 flow_advance_skipped_loop_blocked  completed_node_id=validate
02:48:33 hub_reinvoke_blocked
```

`maybeAutoAdvanceVibeSprintBoundary` correctly refused to arm mid-flight
(`hasRunningSprintStep` veto, the BUG-619 guard). The audit settle then
fell through to the terminal path — `flow_run_complete` — instead of
deferring the boundary decision until the straggler step settled.

## Defect

The BUG-619 evidence gate (never arm a boundary on unverifiable sprint
work) is correct as a *veto*, but there is no companion *defer*:

1. `maybeAutoAdvanceVibeSprintBoundary` returns false on
   `!vibeSprintEvidenceComplete || hasRunningSprintStep` — silently.
2. The audit settle path treats "boundary not armed" as "plan drained"
   and proceeds to `flow_run_complete` + loop `done`.
3. When the straggler (`validate`) completes, the advance path checks the
   loop — already sealed — and logs `flow_advance_skipped_loop_blocked`.
4. `vibeSprintBoundaryPending` was never armed, so `maybeRepark`'s
   reopen-sweep is the only recovery: `POST /workflow-runs/{id}/resume`
   → repark `vibe_sprint_boundary` → Continue starts the next sprint.

Net effect: a perfectly healthy run terminates at 3/6 tasks purely from
event interleaving — a ~9 s straggler window decided run completion.

## Expected fix direction

- When the boundary veto fires on *transient* evidence (a step still
  RUNNING/PENDING, open cohort, spawn in flight), the settle path must
  **defer, not terminate**: mark the audit DONE, arm a durable
  "boundary-deferred" record, and re-evaluate `maybeAutoAdvance…` on the
  straggler's terminal event instead of running `flow_run_complete`.
- Alternatively gate `flow_run_complete` itself on
  `vibeSprintEvidenceComplete` — a run with plan tasks remaining and any
  non-terminal step must not seal.
- The reopen repark (`maybeReparkVibeSprintBoundary`) already implements
  the correct recovery semantics; the live path needs the same check at
  settle time so reopen is never required.
- Regression test shape: sprint-3 audit settles while a stub validate leg
  is still RUNNING → run must stay running/blocked-boundary, then
  auto-start sprint-4 when the leg completes.
