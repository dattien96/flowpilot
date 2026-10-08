# BUG-655 — `zero_delta_progress` drift detection scores 100 on synthesis/debate hub turns that write no files BY DESIGN, producing `pause_for_human` wedges and hundreds of spurious drift events

- **ID:** BUG-655
- **Severity:** Medium — each event spawns owner-debate traffic and can
  wedge a run; 419 drift events recorded in one run. In vibe mode the
  pause is suppressed, but the events still flood telemetry and trigger
  debates.
- **Status:** FIXED — CA-1236 (2026-10-08): zero_delta_progress suppressed for scan/high_reasoning node classes via drift detector WorkloadClass

## Evidence chain (all live)

1. Synthesis hub turns and owner-debate turns legitimately produce no
   file delta (they route/decide). The drift detector counted these as
   zero-progress turns → score 100 → `pause_for_human` intent on
   flow-parkable nodes.
2. 419 `zero_delta_progress` events in the run; debates spawned to
   remediate "stalls" that were just verdict turns.

## Root cause

The drift detector measures file-delta per turn without node-class
awareness — routing nodes (synthesis hubs, debates, gate re-eval) have
zero expected delta by construction. Related BUG-467 is the inverse
shape (suppressed drift); this is false-positive firing.

## Fix direction

- `F-1` Exempt non-writing node classes (synthesis, debate owner,
  gate-eval, plan nodes) from `zero_delta_progress` — measure them by
  outcome emission instead (a verdict/outcome call IS progress).
- `F-2` If exemption is too broad: score writing turns only, and require
  N consecutive zero-delta WRITING turns before any action.

## Regression coverage

- `TestBug655_SynthesisTurnNoDrift` — hub turn with verdict + no files →
  no drift event.
- `TestBug655_WriterTurnZeroDeltaStillDetected` — coder/scaffold turn
  with no files still fires (guard against over-exemption).
