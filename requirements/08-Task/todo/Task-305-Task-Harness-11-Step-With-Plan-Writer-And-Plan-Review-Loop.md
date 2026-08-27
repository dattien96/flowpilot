# Task-305: Task-Harness 11-Step With Plan Writer And Plan Review Loop

## Metadata

- Document ID: `Task-305`
- Title: `Task-Harness 11-Step With Plan Writer And Plan Review Loop`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-27`
- Last Updated: `2026-08-27`
- Parent Documents: [CP-58: Bug / Task / CP Harness With Plan Artifact And Dual Review Loops](../../07-Coding-Plan/todo/CP-58-Bug-Task-Cp-Harness-Plan-Review-Loop.md)
- Child Documents: `None`
- Related Documents: [CP-55: Flow-First Preflight Contract](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md), [Task-293: Rag-Harness TDD + Review Loop](../../08-Task/done/Task-293-RagHarness-TDD-Test-Signatures-And-Review-Until-Clean-Loop.md), [rag-harness.yaml](../../../apps/local-runner/internal/agentpack/flow-pack/flows/rag-harness.yaml), [Task-304: Dual Back-Edge](../todo/Task-304-Dual-Back-Edge-Validator-And-Edge-Driven-Re-entry.md)
- Replaces: `None`
- Tags: `flow, harness, task, plan-artifact, review-loop, agent-flow-engine`

## AI Quick View

### Summary

- Adds built-in `task-harness.yaml` 11-step: `plan→freeze→context→plan_writer→test_signatures→plan_reviewer→plan_synthesis --continue--> plan_writer --done--> implement→validate→reviewer→synthesis→audit`.
- `plan_writer` is `agent.code reinvoke` that writes `Task-*.md` (SS-13 §5.1 + `P-*`/`Risks`/`Acceptance Check`) as `file_artifact`; `test_signatures` stays `agent.code tester`; plan review (`plan_reviewer` cohort:plan + `plan_synthesis` hub) must approve before code loop starts.
- Keeps `rag-harness` byte-identical as `bug-harness` fallback; `task-harness` is `selectableIn:flow cloneable:true mirror:required`.

### Current Ask

- Ship the Tier-2 harness so a normal `Task` (1-3d, needs HLD/LLD) gets a reviewed plan artifact before `implement`.

### Key Decisions

- `T-1` `plan_writer` is `agent.code` (not `agent.delegate`) so `ValidateFlowSafetyTopology` binds `contract.freeze` and enforces `acceptance_nodes` inclusive of `plan_synthesis`.
- `T-2` `test_signatures` sits *between* `plan_writer` and its review so reviewer sees `Task.md` + signatures together; review criteria = `SS-13` doc contract + `R1/R2/R3` analog (missing `AC`, missing `DeclaredPaths`, single happy-path matrix, contradicts `CA`).
- `T-3` `bug-harness` kept as alias/clone of current `rag-harness` — no edge/prompt change — so Bug scope pays zero extra turns.
- `T-4` Two `hub.inline` nodes (`plan_synthesis` + `synthesis`) require Task-304's `activeHubNodeID` tracking; do not rely on `hubInlineNodeID` first-match for step marking in this flow.

### Constraints

- Depends on Task-304 dual `continue/back` landing (including `activeHubNodeID` hub-inline tracking for the two `hub.inline` nodes).
- Additive only; do not edit `review-loop.yaml`/`context-coding-review-synthesis.yaml`.
- `preflight_contract_plan` stays as contract-defined JSON (`agents/contract-planner.md`); `plan_writer` is the real plan.

### Open Questions

- `Q-1` `plan_writer` agent: reuse `coder.md` or new `planner.md` dedicated to spec authoring?
- `Q-2` Plan review cohort size 1 vs 2 — default 1 for cost.

### Source Refs

- `CP-58` `P-2`, `P-3`, §3.1 task-harness table; `rag-harness.yaml:43-148`; `pack.go:794`, `flow_executor.go:904`; `Task-293` inventory pattern; `SD-23` D-11 `file_artifact`.

## 1. Goal

Deliver a selectable `task-harness` flow that produces a reviewed `Task-*.md` plan artifact (with `P-*`/`Source Refs`/`Risks`/`Acceptance Check`) and empty test signatures *before* `implement` runs, so the code loop starts from an approved plan.

## 2. Parent Links

- coding plan: `CP-58` `P-2`, `P-3`, §3.1
- tech design: `SD-19`, `SD-18`, `SD-23` (file_artifact)
- system spec: `SS-13` (doc contract), `SS-15`, `SS-16`, `SS-14`
- specific upstream ids: `CP-58 §3.1 task-harness`, `rag-harness.yaml` 12-node sketch

## 3. Trigger

Today `CP→Task-300..303` is manual 30-60 min per Task; `rag-harness` has no plan doc gate — coder starts immediately after `context`. A `Task` that needs HLD/LLD is therefore unprotected against plan drift before code is written.

## 4. Exact Change

- `T-1` Add `apps/local-runner/internal/agentpack/flow-pack/flows/task-harness.yaml` — 12 nodes incl. `audit` (11 steps + terminal), edges as §3.1: `plan→freeze→context→plan_writer→test_signatures→plan_reviewer→plan_synthesis --continue(back)--> plan_writer --done(forward)--> implement→validate --done--> reviewer→synthesis --done--> audit→done`, `validate --continue(back)--> implement`. `acceptance_nodes:[plan_synthesis, validate, synthesis, audit]`, `mode:flow`, `builtin: {editable:false, selectableIn:[flow], cloneable:true, mirror:{required:true, source:builtin}}`, `policy:{cap:3, onCap:escalate, extendBy:2, extendMax:2}`.
- `T-2` Add `apps/local-runner/internal/agentpack/flow-pack/prompts/plan-task.md` — instructs to read `preflight_contract_plan` intent + `context` package + last `CA` history, write exactly one `requirements/08-Task/todo/Task-xxx-*.md` per `SS-13` + `FORMAT-REFERENCE-TASK.md` §1-§8 with `Metadata`, `AI Quick View`, `Goal`, `Parent Links`, `Trigger`, `Exact Change`, `Touched Areas`, `Acceptance Check`, `Out of Scope`, `Completion Notes`; include `P-*` traceability, `DeclaredPaths`, `Risks`, `Source Refs`. Enforce `file_artifact` OUTPUT path via `declared_paths`. (Task files have no `DOD` section per format — the acceptance gate lives in `Acceptance Check`.)
- `T-3` Add `apps/local-runner/internal/agentpack/flow-pack/prompts/review-plan.md` — gate: `AC` completeness, `P-*` traceability, single happy-path matrix (needs `R3` near-miss, mirroring `review-safe-fix-contract.md`), `DeclaredPaths` coverage, contradicts `CA` history; `test_signatures` not empty/stubbed; approve via `submit_review_outcome` only when all clean (record-only per `CP-53`).
- `T-4` Update `apps/local-runner/internal/agentpack/flow-pack/manifest.yaml` — add `flows/task-harness.yaml` to `flows`, add `prompts/plan-task.md` + `prompts/review-plan.md` to `prompts` (keep `cloneable/editable` sync per `pack.go:769`).
- `T-5` Optional `flows/bug-harness.yaml` as byte-identical clone of `rag-harness.yaml` with `id: bug-harness` and label, or keep `rag-harness` and alias — whichever avoids duplicating drift.
- `T-6` Add `apps/local-runner/internal/agentpack/task_harness_pack_test.go` — `TestTaskHarnessPackTopology` asserts 12 nodes, 2 `continue/back` with different `From`, `acceptance_nodes` includes `plan_synthesis`, `plan_writer` promptTemplate = `prompts/plan-task.md`, `plan_reviewer` `cohort:plan join:all`, exactly 2 `hub.inline` nodes (`plan_synthesis`, `synthesis`), and no `test_signatures` inside the plan loop (it sits between `plan_writer` and `plan_reviewer`, exactly once).

## 5. Touched Areas

- files: `apps/local-runner/internal/agentpack/flow-pack/flows/task-harness.yaml` (new), `flow-pack/prompts/plan-task.md` (new), `flow-pack/prompts/review-plan.md` (new), `flow-pack/manifest.yaml`, `apps/local-runner/internal/agentpack/task_harness_pack_test.go` (new), optional `flows/bug-harness.yaml`
- modules: agent-pack loader; flow executor (dual-loop path already fixed in Task-304)
- routes: none
- tables: `workflows`/`workflow_steps` (mirrored via `EnsureBuiltinFlowMirrorsWithStore` — def `supabase_workflow_flow_store.go:903`, call `internal/cli/root.go:167`)

## 6. Acceptance Check

- `go test ./internal/agentpack -run TestTaskHarness` green; `LoadBuiltinPack` includes `task-harness` with `selectableIn:flow` and `cloneable:true`.
- `go test ./internal/runner -run TestTaskHarnessDualLoop` (new) — `plan_synthesis continue` reuses `plan_writer` session, `validate continue` reuses `implement` session, `context` stays `DONE` (not reset) via `forwardReachableNodeIDs`.
- `go test ./internal/runner -run TestApplyFlowControlContinueHubRouting` (from Task-304) green against `task-harness` topology — `plan_synthesis` continue and `synthesis` continue route to their own loops.
- Manual `/flow task-harness` writes `Task-*.md` + empty `Test*` signatures, `plan_review` can reject → re-enters `plan_writer` with findings, then code loop completes to `done`; artifact panel shows plan md.

## 7. Out of Scope

- Engine dual-loop fix — Task-304.
- `cp-harness` + `task_splitter` — Task-306.
- `file_artifact` instance seeding / panel parity beyond pack-level binding — Task-307.

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated: `CP-58` `P-2`, `P-3`, §3.1
