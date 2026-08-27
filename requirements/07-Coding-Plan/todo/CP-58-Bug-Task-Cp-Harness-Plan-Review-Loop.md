# CP-58: Bug / Task / CP Harness With Plan Artifact And Dual Review Loops

## Metadata

- Document ID: `CP-58`
- Title: `Bug / Task / CP Harness With Plan Artifact And Dual Review Loops`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-27`
- Last Updated: `2026-08-27`
- Parent Documents: [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [SD-18: Main-Hub Agent Review Loop](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md), [SD-23: Generic Artifact Framework](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SD-21: Change Contract And Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md), [SS-15: Agent Review Loop (Review Until Clean)](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md), [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: [Task-304: Dual Back-Edge Validator And Edge-Driven Re-entry](../../08-Task/todo/Task-304-Dual-Back-Edge-Validator-And-Edge-Driven-Re-entry.md), [Task-305: Task-Harness 11-Step With Plan Writer And Plan Review Loop](../../08-Task/todo/Task-305-Task-Harness-11-Step-With-Plan-Writer-And-Plan-Review-Loop.md), [Task-306: CP-Harness 12-Step With CP Plan And Task Splitter](../../08-Task/todo/Task-306-Cp-Harness-12-Step-With-Cp-Plan-And-Task-Splitter.md), [Task-307: Harness Plan Artifact Types And Panel Parity](../../08-Task/todo/Task-307-Harness-Plan-Artifact-Types-And-Panel-Parity.md)
- Related Documents: [CP-55: Flow-First Preflight Contract](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md), [CP-36: Agent Review Loop And Main Hub Orchestration](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [CP-41: RAG Harness Flow Mode](../../07-Coding-Plan/done/CP-41-RAG-Harness-Flow-Mode.md), [rag-harness.yaml](../../../apps/local-runner/internal/agentpack/flow-pack/flows/rag-harness.yaml), [Task-293: Rag-Harness TDD + Review Loop](../../08-Task/done/Task-293-RagHarness-TDD-Test-Signatures-And-Review-Until-Clean-Loop.md), [CP-57: Opencode Provider Integration](./CP-57-Opencode-Provider-Integration.md)
- Replaces: `None`
- Tags: `flow, harness, bug, task, cp, plan-artifact, review-loop, agent-flow-engine, artifact-framework`
- Feature Keys: `agent-flow-engine`

## AI Quick View

### Summary

- Keeps the current `rag-harness` 9-step as **`bug-harness`** for hotfix/bug (`plan→freeze→context→test_signatures→implement→validate→reviewer→synthesis→audit`, 1 loop `validate→implement`). No cost added for small scope.
- Adds **`task-harness` 11-step** for a normal `Task`: inserts `plan_writer` (reads `plan/freeze/context`, writes `requirements/08-Task/*.md` + `07-Coding-Plan/*.md` fragment as `file_artifact` md) + `test_signatures`, then a **plan review loop** (`plan_reviewer` cohort `join:all` + `plan_synthesis hub.inline` `--continue(back)-->plan_writer`) that must approve plan+signatures before `implement`. This is the real HLD/LLD plan; `preflight_contract_plan:43` stays as `contract-defined` JSON.
- Adds **`cp-harness` 12-step** for a `CP`: `cp_plan_writer` (writes `CP-5x.md`) → `cp_review` loop → `task_splitter` (`context.produce`/`agent.code` reading `CP.md` → writes `Task-30x.md × N` file_artifacts) → `audit` (slice-only default, 8 nodes; coding chain NOT declared). Opt-in `cp-harness-smoke.yaml` (13 nodes) chains `test_signatures→implement→validate→reviewer→synthesis→audit` for the first sliced Task; per-Task coding otherwise runs via `task-harness` separately — avoiding loop-in-loop cost blowup.
- Unblocks the 2-loop shape by fixing the pack validator / `resolveContinueBackEdgeTarget` (`pack.go:845`, `flow_executor.go:910`) from single `when` key to `(from,when)` so `plan loop` and `code loop` can coexist as two `continue/back` edges.

### Current Ask

- Author a coding plan that makes `BUG → 9-step`, `Task → 11-step`, `CP → 12-step` selectable built-ins, eliminates the manual `CP→Task*.md` slicing we do today (`CP-58` itself was sliced manually into `Task-300..303`), and keeps the 3 harnesses additive over `rag-harness` with zero regression to `review-loop`/`context-coding-review-synthesis`.

### Key Decisions

- `P-1` **Three harnesses, not one fat flow.** `bug-harness` (clone of current `rag-harness`, maybe renamed), `task-harness` (9 rag nodes + 3 plan nodes `plan_writer`/`plan_reviewer`/`plan_synthesis` = 12 nodes), `cp-harness` (slice-only 8 nodes; smoke variant 13 nodes: + `task_splitter` + coding chain). SelectableIn `flow`, cloneable. Sharing prompts/agents, not edges, keeps each harness' `cap`/`acceptance_nodes` honest and cost-proportional. Alternative "one 12-step with conditional skip" needs engine-level conditional edges — deferred.
- `P-2` **`preflight_contract_plan` is not the plan.** It stays as `contract-defined` JSON (`agents/contract-planner.md:23`). The *real* plan is `plan_writer` / `cp_plan_writer` (`agent.code`, bounded file_artifact md output) — cost is explicit and reviewable. This fixes the owner's #2 confusion.
- `P-3` **Plan review loop mirrors code review loop** (`P-2`/`P-3` reuse): `plan_reviewer:agent.delegate cohort:plan join:all` → `plan_synthesis:hub.inline join:all tools:[submit_review_outcome]` with single `continue/back` to its writer. Review criteria = `SS-13 §5` doc contract + `R1/R2/R3` analog for plan (missing `AC`, missing `DeclaredPaths`, single happy-path matrix, contradicts `CA` history). Code `test_signatures` sits *between* `plan_writer` and its review so the reviewer sees both.
- `P-4` **CP flow stops after slice by default.** `cp-harness` default terminal is after `task_splitter`+`audit` (plan+slice audit), not after full coding of every sliced Task. Coding uses `task-harness` per Task to keep `cap:3` per unit and avoid a 12-step linear chain that would need `cap:6` and defeat retry scoping. Optional smoke = separate `cp-harness-smoke.yaml` that chains coding of the *first* Task only.
- `P-5` **Dual `continue/back` edges are first-class.** Fix `pack.go:865 ValidateFlowDefinition` duplicate check and `flow_executor.go:910 resolveContinueBackEdgeTarget` to be source-aware (`from+when`). This is the only engine change; everything else is pack YAML + artifact types.
- `P-6` **Plan artifacts are `file_artifact` typed artifacts** (`SD-23 D-11`), not free-form writes. `plan_writer` outputs `file_artifact` instance `plan_md` (OUTPUT, required); `plan_reviewer` reads it as INPUT; `task_splitter` reads `CP.md` INPUT and writes `task_md[]` OUTPUT. Deterministic, validated, and visible in the artifact panel. No new artifact *type* DSL — reuse existing `file_artifact`.

### Constraints

- Additive only over `rag-harness`/`review-loop`/`context-coding-review-synthesis`; do not mutate their proven 1-loop `cap:3` semantics except the narrow validator fix in `P-5`.
- `agent.code` writer semantics (`CP-55 P-1` `ValidateFlowSafetyTopology`) still hold: every writer needs a dominating `contract.freeze`, every `done` path crosses `acceptance_nodes`.
- No new provider branching; harness choice is mode data.
- `Task`/`BugFix` remain deltas per `SS-13` — plan md is the Task's own spec, not a replacement for `CP`.
- Manual `CP→Task` today is ~30-60 min per Task; target harness reduces to <5 min review per plan loop round.

### Open Questions

- `Q-1` Should `cp-harness` by default stop after slice+slice-audit, or chain coding of Task-1 as smoke? (Proposed: stop after slice; smoke is opt-in clone.)
- `Q-2` Plan `file_artifact` store path: `requirements/07-Coding-Plan/todo/CP-*.md` vs `requirements/08-Task/todo/Task-*.md` — per-artifact instance `config.pathTemplate` or writer's `declared_paths`?
- `Q-3` Plan review cohort size: 1 reviewer (like `rag-harness`) vs 2 (correctness+completeness)? Proposed 1 for cost.
- `Q-4` Cap split: `plan cap:3` + `code cap:3` independent or shared `policy.cap:5`? **RESOLVED**: single shared `policy.cap:3` covers both loops — reset scoping via `forwardReachableNodeIDs` (`flow_executor.go:936`) keeps per-loop rounds honest (see §6).

### Source Refs

- `SS-15`, `SS-16` §§7; `SD-18`, `SD-19` §§5-7; `SD-23` D-11; `SD-17`, `SD-21` (CP-55); `CP-36`, `CP-41`, `CP-55`, `CP-57`.
- `rag-harness.yaml:43-148`, `review-loop.yaml`, `context-coding-review-synthesis.yaml`, `pack.go:794 ValidateFlowDefinition`, `flow_executor.go:904 resolveContinueBackEdgeTarget`, `flow_validate_audit_dispatch.go`, `artifact_type_registry.go`.

## 1. Goal

Give FlowPilot a **tiered harness family** that matches actual work size and removes manual `CP→Task` slicing:

| Tier | When | Flow | Steps | What it guarantees |
|---|---:|---|---|---|
| Bug | hotfix, 1 file, already clear `AC` | `bug-harness` 9-step (current `rag-harness` as-is, maybe renamed) | `plan→freeze→context→test_signatures→implement→validate→reviewer→synthesis→audit` | TDD signatures + code review loop |
| Task | feature slice 1-3d, needs HLD/LLD | `task-harness` 11-step | same as Bug + `plan_writer` + `plan_review loop` before `test_signatures` | **Plan artifact reviewed before code** + code loop |
| CP | multi-task initiative | `cp-harness` 12-step (slice-only default 8 nodes; smoke variant 13 nodes) | `cp_plan_writer→cp_review loop→task_splitter→audit` (default; no coding chain — dead `agent.delegate` nodes would spawn as entry nodes). Smoke variant: `...→task_splitter→test_signatures→implement→validate→reviewer→synthesis→audit` | **CP doc reviewed, then auto-sliced into `Task*.md`**; per-Task coding runs via `task-harness` separately |

A user picks the tier in `/flow` picker; the flow writes the correct md artifacts (`CP-*.md`, `Task-*.md`, `BUG-*.md`) via `file_artifact` so the next tier can consume them — no manual copy-paste.

## 2. Input Documents

- [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md)
- [SD-18: Main-Hub Agent Review Loop](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md)
- [SD-23: Generic Artifact Framework](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md)
- [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md)
- [SD-21: Change Contract And Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md)
- [CP-55: Flow-First Preflight Contract](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md)
- [CP-36: Agent Review Loop And Main Hub Orchestration](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md)
- [CP-41: RAG Harness Flow Mode](../../07-Coding-Plan/done/CP-41-RAG-Harness-Flow-Mode.md)

## 3. Implementation Strategy

- **overall approach:**
  - Keep `rag-harness` byte-identical as `bug-harness` (rename or clone). Additive new packs only.
  - Fix the engine's dual-loop limitation first (smallest blast radius), then add pack YAML for `task-harness` and `cp-harness` reusing existing agents/prompts plus 2 new prompts + 1 splitter.
  - Wire `file_artifact` bindings for md outputs so Desktop already renders them (SD-23 panel) with zero new UI except harness picker labels.
  - Mirror `Task-293` additive-test style: every harness YAML ships with pack-level topology tests + a minimal `flow_executor` dual-loop test; no pre-existing test edited except inventory.
- **sequencing logic:**
  1. `P-1` Engine: dual `continue/back` (`pack.go` + `flow_executor.go`).
  2. `P-2` Pack: `task-harness` 11-step YAML + prompts + `manifest.yaml` entry.
  3. `P-3` Pack: `cp-harness` 12-step YAML + `task_splitter` + CP prompts.
  4. `P-4` Artifact: `file_artifact` instances for `plan_md` / `cp_md` / `task_md` + bindings.
  5. `P-5` Docs: `SS-13` contract checks for new harness DODs; optional TUI/WorkflowsSettings harness labels.
  - Each P independently testable; landing order is 1→2→4→3, with 3 gated on 2's dual-loop proof.
- **dependencies:**
  - `agentpack/pack.go` validator + `runner/flow_executor.go` back-edge resolver (for `P-1`).
  - `internal/agentpack/flow-pack/flows/*`, `flow-pack/prompts/*`, `flow-pack/manifest.yaml`, `supabase_workflow_flow_store.go` (mirror sync), `artifact_type_registry.go` (for `P-4`).
  - `review-loop`/`rag-harness` as reference to copy edge semantics.

### 3.1 Topology Sketches (normative)

**task-harness (12 nodes incl. `audit`, 2 loops share no edge):**

| Node | Run | Lifecycle | Behavior | Agent | Cohort/Join |
|---|---|---|---|---|---|
| `preflight_contract_plan` | delegate | once | agent.delegate | contract-planner | — |
| `preflight_contract_freeze` | inline | once | contract.freeze | — | — |
| `context` | inline | once | context.produce | — | — |
| `plan_writer` | delegate | reinvoke | agent.code | coder | — |
| `test_signatures` | delegate | once | agent.code | tester | — |
| `plan_reviewer` | delegate | spawn | agent.delegate | reviewer | cohort:plan join:all |
| `plan_synthesis` | inline | reinvoke | hub.inline | synthesizer | join:all |
| `implement` | delegate | reinvoke | agent.code | coder | — |
| `validate` | inline | once | command.validate | — | — |
| `reviewer` | delegate | spawn | agent.delegate | reviewer | cohort:review join:all |
| `synthesis` | inline | reinvoke | hub.inline | synthesizer | join:all |
| `audit` | inline | once | artifact.audit_draft | — | — |

Edges: `plan→freeze→context→plan_writer→test_signatures→plan_reviewer→plan_synthesis`; `plan_synthesis --continue(back)--> plan_writer` (loop 1), `plan_synthesis --done(forward)--> implement`; `implement→validate --done--> reviewer→synthesis --done--> audit→done`; `validate --continue(back)--> implement` (loop 2) — `synthesis --continue` reuses the same `validate→implement` edge via `forwardReachableNodeIDs` reset covering `implement→validate→reviewer→synthesis`. `acceptance_nodes:[plan_synthesis, validate, synthesis, audit]`.

**cp-harness (slice-only default 8 nodes incl. `audit`; smoke variant 13 nodes, same engine):**

Same as task-harness but replace the plan loop with `cp_plan_writer` (writes `CP-5x.md`) + `cp_reviewer` + `cp_synthesis` (no `test_signatures` inside — a CP has no tests yet) and insert `task_splitter` after it: `cp_synthesis --done--> task_splitter (context.produce, reads CP.md file_artifact, writes `Task-30x.md × N`)`. Default slice-only: `task_splitter --done--> audit --done--> done` — no coding chain declared (dead `agent.delegate` nodes with no incoming forward edge are entry nodes spawned at flow start, `flow_executor.go:97/152`). Smoke: separate opt-in `cp-harness-smoke.yaml` (13 nodes) replaces `task_splitter→audit` with `task_splitter→test_signatures→implement→validate→reviewer→synthesis→audit` for the first sliced Task.

## 4. Work Breakdown

- `P-1` **Engine: allow two `continue/back` edges, make re-entry source-aware.** Files: `apps/local-runner/internal/agentpack/pack.go:845` change `backEdgeSources map[when]from` to `map[when+from]from` (or `map[string]map[string]string`), `apps/local-runner/internal/runner/flow_executor.go:910 resolveContinueBackEdgeTarget` to variadic `(edges, from ...string)` with hub-aware resolution (edge.From==from; else for a hub emitter the back-edge whose From's forward-closure reaches the hub; else first-match), `apps/local-runner/internal/runner/interactive_service.go:1507/2900` pass `rs.activeHubNodeID` (Task-235) into resolver, `apps/local-runner/internal/runner/flow_step_runtime.go:577` guard `hubInlineNodeID` first-match for >1 hub.inline node (`activeHubNodeID` tracking at cohort-join `:4711` + `loopIsAdvancing` sites), `apps/local-runner/internal/agentpack/pack_test.go` dual-loop acceptance test. Guard: existing single-loop flows still load (`Resolve` picks the only `continue/back` regardless of `from`; variadic keeps all pre-existing call sites compiling unchanged).
- `P-2` **`task-harness` 11-step pack.** Files: `flows/task-harness.yaml` (new), `prompts/plan-task.md` (HLD/LLD + SS-13 §5.1 metadata + `AC` + `DeclaredPaths` + `Risks`), `prompts/review-plan.md` (plan+signatures gate: `AC` completeness, `P-*` traceability, single happy-path, contradicts `CA` history, per `SS-13` doc contract), `manifest.yaml` flows/prompts entries, `flows/bug-harness.yaml` (clone of `rag-harness.yaml` if renamed). `acceptance_nodes` includes `plan_synthesis`.
- `P-3` **`cp-harness` pack.** Files: `flows/cp-harness.yaml` (new, slice-only 8 nodes) + `flows/cp-harness-smoke.yaml` (new, opt-in 13 nodes), `prompts/plan-cp.md` (CP §1-§10, `DOD`, `P-*`, `R-*`), `prompts/review-cp.md`, `prompts/task-splitter.md` (read `CP.md` file_artifact INPUT → write `Task-*.md` file_artifacts OUTPUT, one per `P-*`), `manifest.yaml` entries. Default terminal after `task_splitter` + slice `audit` (slice-only, `acceptance_nodes:[cp_synthesis, audit]`); `cp-harness-smoke` chains `task_splitter→test_signatures→implement→validate→reviewer→synthesis→audit` (extends `acceptance_nodes` with `validate, synthesis`).
- `P-4` **Plan artifact bindings.** Files: `internal/agentpack/flow-pack/contexts/*` if needed, `internal/runner/artifact_type_registry.go` `file_artifact` resolver reuse (Task-201/202 path injection), `supabase/migrations/*_add_harness_artifacts.sql` seeding `artifact_types`/`artifact_instances` (`is_builtin=true`) for `plan_md`/`cp_md`/`task_md`, `supabase_workflow_flow_store.go` `recordFromWorkflowRow` binding denormalization. RLS `authenticated` blocked on `is_builtin=true` (mirror `workflows` pattern `SD-23 §3.11`).
- `P-5` **Validation + docs.** Files: `internal/agentpack/task_harness_pack_test.go`, `internal/runner/task_harness_dual_loop_test.go` (plan loop + code loop both reinvoke with session reuse), `requirements/05-System-Specs/SS-13*` DOD note, `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` harness labels (optional).
- `P-6` **Parity + operator.** Wire `skillpack`, TUI `/flow` picker `selectableIn:flow`, `cloneable:true`, `mirror:required`. No provider branching.

## 5. Touched Areas

- **files:**
  - `apps/local-runner/internal/agentpack/pack.go` (`ValidateFlowDefinition` dual back-edge)
  - `apps/local-runner/internal/agentpack/flow-pack/flows/task-harness.yaml` (new), `flows/cp-harness.yaml` (new, slice-only), `flows/cp-harness-smoke.yaml` (new, opt-in), `flows/bug-harness.yaml` (new if renamed)
  - `apps/local-runner/internal/agentpack/flow-pack/manifest.yaml` (flows/prompts)
  - `apps/local-runner/internal/agentpack/flow-pack/prompts/plan-task.md` (new), `prompts/review-plan.md` (new), `prompts/plan-cp.md` (new), `prompts/review-cp.md` (new), `prompts/task-splitter.md` (new)
  - `apps/local-runner/internal/agentpack/flow-pack/agents/*` (reuse `coder.md`/`reviewer.md`/`synthesizer.md`, no edit)
  - `apps/local-runner/internal/runner/flow_executor.go` (`resolveContinueBackEdgeTarget` + `forwardReachableNodeIDs` call sites)
  - `apps/local-runner/internal/runner/interactive_service.go` (`applyFlowControl` continue reset + hub-inline `activeHubNodeID` tracking at cohort-join/`loopIsAdvancing` sites)
  - `apps/local-runner/internal/runner/flow_step_runtime.go` (`hubInlineNodeID` guard for >1 hub.inline node)
  - `apps/local-runner/internal/runner/artifact_type_registry.go`, `supabase_workflow_flow_store.go`
  - `apps/local-runner/internal/agentpack/pack_test.go`, new `*_dual_loop_test.go`
  - `supabase/migrations/*` (artifact seeding if not pack-embedded)
- **modules:** agent-pack loader/validator; flow executor; interactive service; artifact framework; desktop WorkflowsSettings (labels only)
- **database:** `workflows`/`workflow_steps`/`step_artifact_bindings`/`artifact_instances`/`artifact_types` (additive rows); no new tables; `workflows.acceptance_nodes_json` for new harnesses
- **external systems:** none; local file_artifact md outputs + Drive sync via `localFileSessionStore`

## 6. Data or Migration Steps

- **schema:** none required for engine; if `artifact_types` seeding needs migration, add `2026xxxxx_add_harness_artifacts.sql` with `is_builtin=true` rows (service role only).
- **data backfill:** `EnsureBuiltinFlowMirrorsWithStore` (def `supabase_workflow_flow_store.go:903`, call `internal/cli/root.go:167`) syncs new harnesses into `workflows` on next runner start; existing `rag-harness` rows keep `acceptance_nodes:[validate,synthesis,audit]` (no backfill on old runs).
- **config updates:** `task-harness` default `policy.cap:3` per loop (shared `cap:3` covers both via `forwardReachableNodeIDs` scoping); document bug/task/cp tier chooser in `/flow` help text.

## 7. Validation Plan

- **tests to add:**
  - `TestValidateFlowAllowsTwoContinueBackEdges` (plan loop + code loop both `when:continue kind:back` with different `From` passes).
  - `TestResolveContinueBackEdgeIsSourceAware` (given `plan_synthesis→plan_writer` and `validate→implement`, resolver with `from=plan_synthesis` returns `plan_writer`, with `from=synthesis` returns `implement`).
  - `TestTaskHarnessPackTopology` (11 nodes, 2 `continue/back`, `acceptance_nodes` includes `plan_synthesis`, `plan_writer` `file_artifact` bindings).
  - `TestCpHarnessPackTopology` (12 nodes, `task_splitter` INPUT `cp_md` OUTPUT `task_md[]`, 2 loops).
  - `TestTaskHarnessDualLoopReinvoke` (plan `continue` reuses `plan_writer` session; code `continue` reuses `implement` session; `context` keeps `DONE` not reset — `forwardReachableNodeIDs`).
  - `TestTaskHarnessPlanReviewBlocksWithoutArtifact` (no `file_artifact` md → `plan_synthesis` escalate).
  - `TestApplyFlowControlContinueHubRouting` (`plan_synthesis` continue re-enters `plan_writer`; `synthesis` continue re-enters `implement` via the `validate→implement` edge — engine-level, Task-304).
  - `TestCpHarnessSliceOnlyTerminal` (default stops after `task_splitter`+`audit`, `implement` never RUNNING) + `TestCpHarnessSmokeVariant` (variant B runs first-Task coding to `done`).
  - `TestHarnessArtifactBindingRejectsPathOutsideRequirements` (OUTPUT path escaping `requirements/` fails deterministically at binding validation).
  - Inventory: `pack_test.go` count 6→9 analog for new harnesses (like `Task-293` `rag_harness_tdd_review_pack_test.go`).
- **manual checks:**
  1. `/flow bug-harness` with `grok-4.5` still behaves as today (no regression, same `cap:3`).
  2. `/flow task-harness` — `plan_writer` writes `Task-xxx.md` with `P-*` + `Source Refs`, `test_signatures` writes empty signatures, `plan_synthesis` approves, then `implement→validate→reviewer→synthesis→audit` completes to `done` with `CP-58` as source doc.
  3. `/flow cp-harness` — `cp_plan_writer` writes `CP-5x.md`, `cp_review` approves, `task_splitter` writes `Task-30x.md ×2`, `audit` shows file_artifact outputs in artifact panel.
  4. Clone any harness in Desktop, edit `cap`, save — reload preserves `acceptance_nodes` (CP-55 P-1 regression).
- **failure cases:**
  - Plan reviewer `changes_requested` → only `plan_writer`/`test_signatures` reset to `PENDING` (`forwardReachableNodeIDs` from `plan_writer`), `context`/`freeze` stay `DONE` (BUG-286).
  - Second `continue/back` with same `From+When` still rejected by validator (duplicate).
  - `file_artifact` path outside workspace → deterministic fail at artifact binding (SD-23).

## 8. Rollout and Fallback

- **rollout order:** `P-1` engine (dual loop) → `P-2` task-harness → `P-4` artifact bindings → `P-3` cp-harness → `P-5` tests/docs. Each P independently shippable; `P-1` alone is safe (single-loop flows still pass).
- **fallback path:** all additive, harness-selectable. Keep `rag-harness` as fallback `bug-harness`. If dual-loop regresses, revert `task-harness`/`cp-harness` pack entries; engine's single-loop path (when caller passes empty `from`) falls back to first-match. No provider or Drive change.
- **monitoring:** `pack_test.go` full suite; `go test ./internal/runner -run DualLoop|TaskHarness|CpHarness`; TUI F2 step runtime reachability; artifact panel shows `plan_md` outputs.

## 9. Risks

- `R-1` **Dual-loop validator widening breaks existing flows.** Mit: `P-1` guard — only allow second loop when `From` differs; single-loop `rag-harness`/`review-loop` unconditionally pass; existing `rag_harness_live_continue_back_edge_test.go:81` keeps green.
- `R-2` **Loop crosstalk (plan `continue` resetting code nodes).** Mit: `forwardReachableNodeIDs` scoped to re-entry's forward closure (`flow_executor.go:936`); `context`/`freeze` not forward-reachable from `plan_writer` → stay `DONE` (BUG-286 fix).
- `R-3` **Artifact type proliferation.** Mit: reuse `file_artifact` only (SD-23 D-11); no new type DSL; instance config is path template only.
- `R-4` **Cost doubling (plan review + code review).** Mit: tiered harnesses — bug skips plan loop; Task pays +~2 turns but catches plan drift before code; CP pays splitter only, not full coding.
- `R-5` **Manual CP→Task bundle drift.** Mit: `cp-harness` enforces one `CP.md` → N `Task.md` mapping with `config.pathTemplate` and `P-*` traceability; drift caught by plan reviewer.
- `R-6` **Supabase mirror sync drops `acceptance_nodes`.** Mit: reuse CP-55 P-1 fix (`workflows.acceptance_nodes_json` + `SupabaseAdminRepository` carry); add harness rows with `acceptance_nodes` seeded.

## 10. Definition of Done

- [ ] `DOD-1` `ValidateFlowDefinition` accepts `task-harness`/`cp-harness` with two `continue/back` edges (different `From`) and still rejects same-`From` duplicate; `go test ./internal/agentpack -run ValidateFlow` green.
- [ ] `DOD-2` `resolveContinueBackEdgeTarget(edges, from...)` source-aware returns correct re-entry for each loop — hub continue routes by `activeHubNodeID` (`plan_synthesis`→`plan_writer`, `synthesis`→`implement` via the `validate→implement` edge); dual-loop reinvoke reuses `plan_writer` and `implement` sessions independently; `context` not reset — `TestTaskHarnessDualLoopReinvoke` + `TestApplyFlowControlContinueHubRouting` green.
- [ ] `DOD-3` `task-harness` 12-node builtin loads (`LoadBuiltinPack`), shows in `/flow` picker (`selectableIn:flow`), `cloneable:true`, `cap:3`, `acceptance_nodes` includes `plan_synthesis`; `bug-harness`/`rag-harness` byte-identical.
- [ ] `DOD-4` `task-harness` live: `plan_writer` writes `Task-*.md` file_artifact, `test_signatures` writes empty signatures, `plan_review` loop can reject and re-enter `plan_writer` with findings, then `implement→validate→reviewer→synthesis→audit` completes to `done`; artifact panel shows plan md.
- [ ] `DOD-5` `cp-harness` slice-only live (8-node default): `cp_plan_writer` writes `CP-*.md`, `task_splitter` writes `Task-*.md ×N`, plan and slice reviews both pass before `audit`; no coding steps on the timeline (`TestCpHarnessSliceOnlyTerminal` green); `cp-harness-smoke` (13-node opt-in) runs first-Task coding to `done` (`TestCpHarnessSmokeVariant` green); manual `CP→Task` bundling no longer needed for new CPs.
- [ ] `DOD-6` No regression: `review-loop`/`context-coding-review-synthesis`/`rag-harness` flows unchanged, `go test ./internal/runner -run 'TestRAGHarnessLive|TestReviewLoop|TestContextCoding'` green, `domain_hardcode_guard` green.
- [ ] `DOD-7` Docs: `CP-58` approved, `artifact_types` seeded with `plan_md` instances, `change-audit/CA-xxx-bug-task-cp-harness.md` with ledger block.
