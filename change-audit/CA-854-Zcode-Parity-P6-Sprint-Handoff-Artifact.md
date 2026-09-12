# CA-854 — CP-62 P-6: sprint-handoff artifact carries verified decisions across vibe sprints

# ---8<--- flowpilot:change-ledger
feature_key: zcode-parity
source_doc_id: Task-342
change_type: task
summary: vibe-sprint audit writes sprint_handoff.v1 per sprint from verified run state only (done step nodes + captured reviewer verdict rows; fail/blocked rows surface as risks) into requirements/.flowpilot/vibe/handoffs/handoff-sprint-N.yaml; the next sprint's entry prompt carries the previous handoff verbatim (graceful fallback when missing); file is JSON (valid YAML 1.2 subset) to avoid a new module dependency; handoff write is imperative at audit-DONE instead of a YAML output binding to avoid racing the per-turn r-artifact-output gate
# --->8---

## Why

Between vibe sprints, decisions (why X over Y, what was deferred and why) lived only in opaque chat summaries — the next sprint could silently reverse them. CP-62 D-6: the audit hub records what/why from verified evidence; the CP-49 hard-ceiling rule applied to runtime.

## Change

- `runner/artifact_type_registry.go`: `ArtifactTypeSprintHandoff = "sprint_handoff.v1"`.
- `runner/sprint_handoff.go` (new): `SprintHandoffV1`/`SprintDecision`/`WeakenedTest`; `emitSprintHandoff` (verified inputs: vibeSprintIndex, vibeTaskName, DONE step nodes via the workflow store, `lastFlowVerdicts`); `previousSprintHandoffContext`; `sprintHandoffPath`.
- `runner/interactive_service.go`: `interactiveRun.lastFlowVerdicts` — verdict rows captured typed from `FlowControlInput.Payload["verdicts"]` on any flow-control application.
- `runner/vibe_cp.go`: `vibeSprintDecision.Sprint`; `maybeChainVibeSprint` emits the handoff when audit completes (best-effort, never blocks the chain); `maybeStartNextVibeSprint` composes the entry prompt as task + previous handoff.

## Tests

`runner/sprint_handoff_test.go` (4): valid write at the exact handoff path with decisions/risks mapped from verdict rows (fail row → risk, never silent done); next sprint consumes the previous handoff; missing file → graceful empty fallback (incl. sprint 1); no verdict rows → zero invented decisions/risks. Vibe regression (Task-321/323/326/328/329 + CP-62 patterns) and agentpack suite green.

## Providers

Case 1 provider-agnostic — the handoff is runner-local file I/O over already-parsed typed rows; no adapter involvement.

## Prior claims intact

CA-791/CA-793 (vibe join/checkpoint) and CA-853 (profiles) untouched — the handoff rides the entry prompt, not the profile source set; BUG-231/234 settle semantics untouched; the sprint chain's existing guards (boundary pending/declined/start-in-flight) preserved verbatim.
