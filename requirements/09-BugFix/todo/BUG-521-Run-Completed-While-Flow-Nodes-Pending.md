# BUG-521 — Outer run status `completed` while flow nodes remain WAITING/PENDING

Status: OPEN (live-found, root cause recorded; fix not yet applied)
Filed: 2026-09-26 (round-5 live test, run-60145)
Related: BUG-520 (the wedge that produced the state), BUG-507
(`completed` withheld while flow loop open — this is a sibling seam)

## Symptom (live run-60145)

After the BUG-520 wedge (hub_stalled park orphaned the armed
gate-reprompt child run-60174), operator `continue` dispatched a plain
hub turn which settled and stamped the run `status=completed`. Durable
step log at that moment:

```
problem_scout      DONE
parallel_rollout   DONE
candidate-a        DONE
candidate-b        WAITING_USER_APPROVAL   ← orphaned, pending_gate_code_paths armed
tournament_arbiter PENDING
merge_and_audit    PENDING
```

So the run reported a terminal `completed` while a required tournament
node was pending and a cohort child was still actionable — a dishonest
terminal surface. (BUG-507's fix withholds `completed` while the *flow
loop* is open; here the loop had gone `blocked`/`running`-post-continue
and the settle path stamped `completed` anyway.)

## Root cause (preliminary)

The turn-settle finalizer evaluates the run-level status from the loop
state and turn outcome, not from the flow graph's required-node
completeness. When the hub turn settled after the continue, nothing
re-checked that `candidate-b`/`tournament_arbiter`/`merge_and_audit`
were non-terminal — or that the flow graph had no path forward without
them. Related: `parkFlowForAwaitingUser` wiped the child's armed
reprompt intent (BUG-520 casualty) so the child could never reach
terminal, and no re-drive path re-arms it on unpark.

## Expected

A run must not surface `completed` while its flow graph has required
nodes in PENDING/WAITING or any non-terminal actionable child — fail
closed to `blocked`/`running` with the honest gate reason surfaced.

## Suggested seam

Run-status stamping should consult the durable step-transition log for
required nodes, or the loop terminalization should mark `completed` only
after the flow engine confirms all required nodes terminal (reusing the
BUG-507 open-loop withhold machinery).

## Live evidence

- run-60145 sessions.ndjson: `status: completed` with
  `loop.status` previously `blocked/hub_stalled`; step log shows
  `candidate-b WAITING_USER_APPROVAL`, `tournament_arbiter PENDING`,
  `merge_and_audit PENDING` at settle time (turns turn-64617/64628).
- Second `continue` unblocked the loop (`running`) but never re-drove
  the orphaned child — the wedge persisted until the run was abandoned.
