# BUG-562 — Sub-flow re-resolve wipes step statuses: done nodes show "pending" in the timeline

- **ID:** BUG-562
- **Severity:** Medium (timeline lies about progress; also opens a narrow window where in-flight-leg checks — BUG-560/561 defer — are blinded)
- **Status:** open
- **Found:** live run-69320 (Task-024 sprint, 2026-10-02 ~13:39)

## Symptom

After the vibe-owner-debate sub-flow resolved and the sprint flow was
re-resolved, the step timeline showed **all 10 sprint nodes as `pending`**
(`0/10 steps`) even though `preflight_contract_plan`, `preflight_contract_freeze`,
`context`, and `tdd` had genuinely completed (contract was frozen at
13:09:52, TDD turn-71186 produced output and went through two debate rounds).

Screenshot evidence: 2026-10-02 13:39:07 — left step list shows
`preflight_contract_plan … audit` all `pending` while `tdd` child run is
live on a remediation turn.

## Root cause

`reseedFlowStepRuntime(parentRunID, nodes)` in `flow_step_runtime.go:104`
**replaces the whole step list with one `StepStatusPending` row per node**,
unconditionally. It is called on every flow resolution:
`flow_executor.go:175`, `flow_executor.go:631`, `vibe_cp.go:1029`.

When a sub-flow (vibe-owner-debate) resolves on the same parent run, the
step list is reseeded to the debate topology — fine — but when the sprint
flow re-resolves afterwards, every previously-completed sprint step is
reset to `pending` too. Statuses self-heal only when the owning leg
settles again (child completion re-stamps `DONE`), so a reseeded
`RUNNING`/`DONE` step lies until then.

## Secondary effect (why it matters beyond cosmetic)

- `vibeSprintEvidenceComplete` reads `lookupFlowStepStatus(coder) != DONE`
  → a wiped coder step makes audit believe the coder leg never ran →
  false "sprint evidence incomplete" escalate.
- `hasRunningSprintStep` (BUG-561) only sees `RUNNING` — a reseed mid-leg
  blanks the in-flight signal → audit may escalate+park and cancel a live
  leg (the BUG-560 kill pattern through the reseed hole).

## Expected fix direction

Merge-preserve reseed: keep existing per-node status/timestamps for node
IDs that already have rows; only insert rows for new nodes (and drop rows
for nodes no longer in the topology). The resume variant
(`reseedFlowStepRuntimeForResume`) already differentiates statuses — the
same semantics apply to mid-run re-resolution.

## Repro sketch

`ca1096`-style fixture: seed sprint nodes, set `coder`/`tdd` DONE, call the
flow-resolution path again on the same run → assert statuses preserved
(currently reset to `pending`).
