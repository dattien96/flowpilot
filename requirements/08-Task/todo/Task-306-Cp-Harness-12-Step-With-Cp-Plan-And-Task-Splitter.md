# Task-306: CP-Harness 12-Step With CP Plan And Task Splitter

## Metadata

- Document ID: `Task-306`
- Title: `CP-Harness 12-Step With CP Plan And Task Splitter`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-27`
- Last Updated: `2026-08-27`
- Parent Documents: [CP-58: Bug / Task / CP Harness With Plan Artifact And Dual Review Loops](../../07-Coding-Plan/todo/CP-58-Bug-Task-Cp-Harness-Plan-Review-Loop.md)
- Child Documents: `None`
- Related Documents: [CP-55: Flow-First Preflight Contract](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md), [Task-293: Rag-Harness TDD + Review Loop](../../08-Task/done/Task-293-RagHarness-TDD-Test-Signatures-And-Review-Until-Clean-Loop.md), [Task-305: Task-Harness 11-Step](../todo/Task-305-Task-Harness-11-Step-With-Plan-Writer-And-Plan-Review-Loop.md)
- Replaces: `None`
- Tags: `flow, harness, cp, splitter, plan-artifact, agent-flow-engine`

## AI Quick View

### Summary

- Adds built-in `cp-harness.yaml` **slice-only (8 nodes)** default: `cp_plan_writer` writes `CP-5x.md` (CP §1-§10 + `P-*`/`R-*`/`DOD`) → `cp_review` loop (`cp_reviewer`+`cp_synthesis` `--continue-->cp_plan_writer`) → `task_splitter` reads `CP.md` file_artifact INPUT and writes `Task-30x.md × N` file_artifact OUTPUTs (one per `P-*`) → `audit→done`. No coding chain in the default flow (dead `agent.delegate` nodes would be spawned as entry nodes at flow start).
- Reuses `task-harness` prompts/nodes; only `cp_plan`/`splitter` are new. Default terminal is slice-only so per-Task coding runs via `task-harness` separately; a separate opt-in `cp-harness-smoke.yaml` (13 nodes) chains coding of the first sliced Task.

### Current Ask

- Ship the Tier-3 harness that automates today's manual `CP-57 → Task-300..303` slicing and enforces `CP.md → Task.md` traceability.

### Key Decisions

- `T-1` `task_splitter` is `context.produce` (deterministic, no provider call) reading `CP.md` file_artifact INPUT — cheaper than `agent.code` and keeps `CP.md` parse pure; fallback to `agent.code` if LLM parsing of `P-*` is needed (hybrid: `context.produce` extracts `P-*` blocks, `agent.code` formats `Task.md`).
- `T-2` CP flow default stops after `task_splitter` + slice `audit` (plan+slice), not full coding of every sliced Task — avoids `cap` sharing across many tasks and keeps cost `cap:3` per tier.
- `T-3` Opt-in smoke = separate `flows/cp-harness-smoke.yaml` (13 nodes) chaining `task_splitter→test_signatures→implement→validate→reviewer→synthesis→audit` of the first sliced Task; documented as variant, not default. (Cannot be a clone-with-edge-replacement: the slice-only flow must not declare the coding-chain nodes at all — see T-1.)
- `T-4` Two `hub.inline` nodes (`cp_synthesis` + `synthesis`) require Task-304's `activeHubNodeID` tracking; do not rely on `hubInlineNodeID` first-match for step marking in this flow.

### Constraints

- Depends on Task-304 (dual loop) and Task-305 (plan loop shape), including `activeHubNodeID` hub-inline tracking for the two `hub.inline` nodes.
- Additive over `task-harness`; do not duplicate its prompts — reuse `review-plan.md` for CP review with CP-specific `plan-cp.md`.
- `file_artifact` bindings are `is_builtin=true` seeded — RLS blocks `authenticated` writes.

### Open Questions

- `Q-1` `task_splitter` impl: `context.produce` vs `agent.code` — which reliably parses `CP §4 P-*` into `Task.md` per `FORMAT-REFERENCE-TASK.md`?
- `Q-2` Number of Tasks `N`: fixed by `CP` `P-*` count (e.g. 4) or model decides 2-5 within cap?

### Source Refs

- `CP-58` `P-4`, §3.1 cp-harness sketch; `rag-harness.yaml`, `task-harness.yaml` (from Task-305) as template; `SD-23` D-11 file_artifact; `artifact_type_registry.go`; `supabase_workflow_flow_store.go`.

## 1. Goal

Automate `CP → Task` decomposition: a CP run writes a reviewed `CP-5x.md`, a `task_splitter` emits `Task-30x.md × N` each traceable to one `P-*`, and the flow audits the slice before any per-Task coding starts.

## 2. Parent Links

- coding plan: `CP-58` `P-4`, §3.1 cp-harness
- tech design: `SD-19`, `SD-18`, `SD-23` (file_artifact), `SD-21`
- system spec: `SS-13` (doc contract), `SS-15`, `SS-16`
- specific upstream ids: `CP-58 §3.1 cp-harness`, `Task-305` plan loop

## 3. Trigger

`CP-58` itself was sliced manually into `Task-300..303` (4 Tasks) by hand — same 30-60 min per Task as every CP. Without `cp-harness`, every new CP repeats this manual bundling and risks `P-*`/`Task` drift.

## 4. Exact Change

- `T-1` Add `apps/local-runner/internal/agentpack/flow-pack/flows/cp-harness.yaml` — **8 nodes incl. `audit` (7 steps + terminal)**: `preflight_contract_plan→preflight_contract_freeze→context→cp_plan_writer→cp_reviewer→cp_synthesis --continue(back)--> cp_plan_writer --done(forward)--> task_splitter→audit→done`. The CP plan loop contains exactly `{cp_plan_writer, cp_reviewer, cp_synthesis}` — **no `test_signatures` inside** (a CP has no tests yet; `test_signatures` belongs to per-Task coding only, per `CP-58 §3.1`). **The coding chain MUST NOT be declared in the slice-only flow**: an `agent.delegate` node with no incoming forward edge is an entry node and is spawned at flow start (`flow_executor.go:97/152` spawns ALL `entryDelegateNodes`), and any incoming forward edge would auto-advance into coding — both break slice-only. Variant B (smoke) is a **separate opt-in flow file** `flows/cp-harness-smoke.yaml` (13 nodes incl. `audit`): same preflight + cp plan loop, then `task_splitter→test_signatures→implement→validate→reviewer→synthesis→audit→done` for the first sliced Task. `acceptance_nodes:[cp_synthesis, audit]` (smoke extends with `validate, synthesis`).
- `T-2` Add `apps/local-runner/internal/agentpack/flow-pack/prompts/plan-cp.md` — instructs to write `requirements/07-Coding-Plan/todo/CP-5x-*.md` per `FORMAT-REFERENCE-CP.md` §§1-10 with `DOD`, `P-*`, `R-*`, `Source Refs`, `Touched Areas`.
- `T-3` Add `apps/local-runner/internal/agentpack/flow-pack/prompts/review-cp.md` — gate: `§10 DOD` measurable, `P-*` actionable+ordered, `§5 Touched Areas` exhaustive, `R-*` mitigations non-trivial, contradicts `CA` history check.
- `T-4` Add `apps/local-runner/internal/agentpack/flow-pack/prompts/task-splitter.md` — reads `CP.md` file_artifact (INPUT `cp_md`) via `file_artifact` resolver, splits each `P-*` into one `requirements/08-Task/todo/Task-*.md` per `FORMAT-REFERENCE-TASK.md` with `Parent Documents: CP-5x` + `P-*` traceability, `file_artifact` OUTPUT `task_md[]` (one per `P-*`, pathTemplate `Task-{{idx}}-{{slug}}.md`).
- `T-5` Update `apps/local-runner/internal/agentpack/flow-pack/manifest.yaml` — add `flows/cp-harness.yaml` + 3 prompts to `prompts`.
- `T-6` Add `apps/local-runner/internal/agentpack/cp_harness_pack_test.go` — `TestCpHarnessPackTopology` asserts **8 nodes**, plan loop = exactly `{cp_plan_writer, cp_reviewer, cp_synthesis}` (no `test_signatures` anywhere in the slice-only flow; no `agent.code` node other than `cp_plan_writer`), `cp_plan_writer` `file_artifact` OUTPUT, `task_splitter` INPUT `cp_md` OUTPUT `task_md[]`, 1 `continue/back` (plan loop only), `acceptance_nodes:[cp_synthesis, audit]`, `cp-harness` selectable; `TestCpHarnessSmokePackTopology` asserts `cp-harness-smoke.yaml` = 13 nodes with the full coding chain and 2 `continue/back` with different `From`.
- `T-7` Do NOT seed `artifact_types`/`artifact_instances` here — instance seeding for `cp_md`/`task_md` is owned by Task-307 (service role, `is_builtin=true`); this task only declares the bindings in YAML (T-1) and asserts them in pack tests (T-6).

## 5. Touched Areas

- files: `apps/local-runner/internal/agentpack/flow-pack/flows/cp-harness.yaml` (new, slice-only 8 nodes), `flows/cp-harness-smoke.yaml` (new, opt-in 13 nodes), `flow-pack/prompts/plan-cp.md` (new), `prompts/review-cp.md` (new), `prompts/task-splitter.md` (new), `flow-pack/manifest.yaml`, `apps/local-runner/internal/agentpack/cp_harness_pack_test.go` (new)
- modules: agent-pack loader; artifact framework (file_artifact resolver); desktop WorkflowsSettings (harness label)
- routes: none
- tables: `workflows`/`workflow_steps`/`step_artifact_bindings`/`artifact_instances` (additive rows if seeded)

## 6. Acceptance Check

- `go test ./internal/agentpack -run TestCpHarness` green; `LoadBuiltinPack` includes `cp-harness` (8-node slice-only) + `cp-harness-smoke` (13-node) with `cloneable:true`.
- `go test ./internal/runner -run TestCpHarnessSliceOnlyTerminal` (new) — default flow reaches `done` after `task_splitter`+`audit`; step timeline contains no `implement`/`validate`/`reviewer`/`synthesis` steps and no entry spawn beyond `preflight_contract_plan`; `TestCpHarnessSmokeVariant` — `cp-harness-smoke` runs first-Task coding to `done`, `cp_synthesis continue` reuses `cp_plan_writer`, `validate continue` reuses `implement`.
- Manual `/flow cp-harness` with `CP-58` as source: writes `CP-5x.md` + `Task-30x.md ×2` visible in artifact panel, `cp_review` can reject → re-enters `cp_plan_writer`, then `audit` to `done` without coding (slice-only default).

## 7. Out of Scope

- Dual-loop engine fix — Task-304.
- `task-harness` 11-step — Task-305.
- Full coding of every sliced Task in one flow — deferred to per-Task `task-harness` runs; smoke path is the separate opt-in `cp-harness-smoke.yaml` (doc + pack test only, not wired into `/flow` picker by default).

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated: `CP-58` `P-4`, §3.1
