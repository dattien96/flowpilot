# BUG-464: Stop leaves inline-hub flow step stamped RUNNING forever on a cancelled run

- status: done
- found: live run-47170 (B-51-6 crash-recovery drill, vibe-ingest)
- fixed_by: CA-961
- tests: internal/runner/bug464_stop_hub_running_sweep_test.go

## Symptom (live)

Sequence on run-47170:

1. `kill -9` mid-`ss_converter` child turn (run-47631 in flight).
2. Restart + `POST resume` → reconstruct normalized run → `cancelled`,
   converter node → CANCELED, and parked a `resumeFrom: ingest_reader`
   gate — all correct.
3. `gate-decision ok` → re-drive: `ss_converter` RUNNING again, new child
   run-47655 spawned and completed — correct.
4. Operator `interrupt` landed right after the converter-completion join.
   Final durable step log:

   ```
   ss_converter DONE     10:45:18.104632
   ss_converter DONE     10:45:18.104893   (double stamp, harmless)
   ss_validator RUNNING  10:45:18.105014
   ```

   `ss_validator` (the vibe-ingest `run: inline` hub node) stayed RUNNING
   on a `cancelled` run — permanently, in both the in-memory step-runtime
   and the durable `*-step-transitions.ndjson`. Sidebar shows a spinning
   node on a dead run; nothing ever reconciles it.

## Root cause

`stopAgentLoop` terminalizes the run and cancels each *cohort member's*
node via `m.label` stamps (the "cancelledCohort" block). The inline hub
node is not a cohort member — it got its RUNNING stamp from the cohort
join's `hubID … && loopIsAdvancing` branch while the loop was still
advancing, an instant before Stop sealed the loop. Once sealed, no code
path ever writes another transition for that node: `loopSealedForReinvoke`
blocks the hub reinvoke that would have eventually re-stamped it.

Replay-side normalization can't help either — `applyStepTransitionReplay`
only normalizes a *persisted* RUNNING line on the next resume; the RUNNING
here was written durably at stop time and is simply re-read as truth.

## Fix

In `stopAgentLoop`, after the cohort-member block: for flow-engine-driven
runs, sweep `LoadRunSteps` and stamp CANCELED on every non-terminal,
non-PENDING row (RUNNING, WAITING_USER_APPROVAL). PENDING stays PENDING
(never started — same convention as the restart evidence-walk); terminal
rows untouched. Mirrors `markFlowRunComplete`'s iteration shape.

## Provider parity

Provider-agnostic: the sweep reads/writes step-runtime rows and the
transition log — no provider path touched. Live repro happened on devin;
the mechanism (cohort join stamp → stop seal → stranded RUNNING) is
identical for claude/codex/grok children because it lives in the shared
stop path.

## Notes

- Also observed (not fixed here): the join emits a duplicate DONE stamp
  for the completing member (~0.3ms apart, two code paths stamp the same
  node). Harmless last-wins append, but the transition log grows a line.
