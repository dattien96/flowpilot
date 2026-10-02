# CA-1119 — BUG-568: restart reconstruction drops parked sprint step rows

## Why

Live run-100368: after a restart with the debate overlay mounted,
`resumedFlowStepRows` rebuilt step rows from `activeFlowNodes` only — which
is the owner-debate graph at that moment. Sprint nodes lived in
`vibeParkedNodes`, so no row existed for them; transition-log replay skipped
every sprint transition (`byID[nodeID] == nil → skip`), and the later
debate-restore reseed merged against empty rows — a previously DONE `coder`
came back PENDING. `vibeSprintEvidenceComplete` then read false and the audit
auto-finalize path refused, wedging the sprint at `blocked_missing_feature_key`.

## What changed

`apps/local-runner/internal/runner/interactive_resume.go`:

- `resumedFlowStepRows` unions `activeFlowNodes` with `vibeParkedNodes` when
  reconstructing step rows. Parked sprint nodes get rows, transition replay
  restores their durable statuses, and the later debate-restore merge sees
  the true pre-restart state instead of reseeding blind.

## Invariant

Durable transition state is authoritative — reconstruction must build rows
for every node that owns durable transitions, including parked subgraphs,
before replay is allowed to restore statuses.

## Tests

`bug567_568_debate_restart_test.go` (red → green):

- `TestBUG568ResumeSeedsParkedSprintSteps` — restart mid-debate preserves
  the sprint `coder` step's DONE status through reconstruction + restore
