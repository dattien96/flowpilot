# Task-306: CP-Harness 12-Step With CP Plan And Task Splitter

## Metadata

- Document ID: `Task-306`
- Title: `CP-Harness 12-Step With CP Plan And Task Splitter`
- Phase: `task`
- Status: `inprogress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-27`
- Last Updated: `2026-09-01`
- Parent Documents: [CP-58: Bug / Task / CP Harness With Plan Artifact And Dual Review Loops](../../07-Coding-Plan/todo/CP-58-Bug-Task-Cp-Harness-Plan-Review-Loop.md)
- Child Documents: `None`
- Related Documents: [CP-55: Flow-First Preflight Contract](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md), [Task-293: Rag-Harness TDD + Review Loop](../../08-Task/done/Task-293-RagHarness-TDD-Test-Signatures-And-Review-Until-Clean-Loop.md), [Task-305: Task-Harness 11-Step](../todo/Task-305-Task-Harness-11-Step-With-Plan-Writer-And-Plan-Review-Loop.md)
- Replaces: `None`
- Tags: `flow, harness, cp, splitter, plan-artifact, agent-flow-engine`

## AI Quick View

### Summary

- Adds built-in `cp-harness.yaml` **slice-only (7 nodes)** default: `plan (Scout)→context (Draft Context)→cp_plan_writer` (writes `CP-5x.md`) → `cp_review` loop (`cp_reviewer`+`cp_synthesis` `--continue-->cp_plan_writer`) → `task_splitter` (reads `CP.md` file_artifact INPUT and writes `Task-30x.md × N` file_artifact OUTPUTs) → `audit→done`. No `freeze`/`test_signatures` or coding chain in the default flow (slice-only planning; per-Task execution runs via `task-harness`).
- Reuses `task-harness` prompts/nodes; only `cp_plan`/`splitter` are new. Default terminal is slice-only so per-Task coding runs via `task-harness` separately; a separate opt-in `cp-harness-smoke.yaml` (13 nodes) chains `freeze→test_signatures→implement→validate→reviewer→synthesis→audit` for coding of the first sliced Task.

### Current Ask

- Ship the Tier-3 harness that automates today's manual `CP-57 → Task-300..303` slicing and enforces `CP.md → Task.md` traceability.

### Key Decisions

- `T-1` `task_splitter` is `context.produce` (deterministic, no provider call) reading `CP.md` file_artifact INPUT — cheaper than `agent.code` and keeps `CP.md` parse pure; fallback to `agent.code` if LLM parsing of `P-*` is needed (hybrid: `context.produce` extracts `P-*` blocks, `agent.code` formats `Task.md`).
- `T-2` CP flow default stops after `task_splitter` + slice `audit` (plan+slice), not full coding of every sliced Task — avoids `cap` sharing across many tasks and keeps cost `cap:3` per tier. Uses `context.produce` before `cp_plan_writer` to package broad repo context for architecture authoring; no `freeze`/`test_signatures`/`coding` in slice-only.
- `T-3` Opt-in smoke = separate `flows/cp-harness-smoke.yaml` (13 nodes) chaining `task_splitter→freeze→test_signatures→implement→validate→reviewer→synthesis→audit` of the first sliced Task; documented as variant, not default. (Cannot be a clone-with-edge-replacement: the slice-only flow must not declare the coding-chain nodes at all — see T-1.)
- `T-4` Two `hub.inline` nodes (`cp_synthesis` + `synthesis`) require Task-304's `activeHubNodeID` tracking; do not rely on `hubInlineNodeID` first-match for step marking in this flow.
- `T-5` **Model Tiering for CP:** `preflight_contract_plan` (Scout) runs with a Fast/Cheap model tier and Broad Context for initial architecture exploration; `cp_plan_writer` and `cp_reviewer` run with the **Highest Reasoning model** (e.g. Claude 3.7 Sonnet, o3, Grok 4.5 high) to author and gate the `CP-*.md` architecture document and oversee `task_splitter` decomposition.

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

- `T-1` Add `apps/local-runner/internal/agentpack/flow-pack/flows/cp-harness.yaml` — **7 nodes incl. `audit` (6 steps + terminal)**: `preflight_contract_plan→context→cp_plan_writer→cp_reviewer→cp_synthesis --continue(back)--> cp_plan_writer --done(forward)--> task_splitter→audit→done`. The CP plan loop contains `{context, cp_plan_writer, cp_reviewer, cp_synthesis}` — **no `test_signatures` and no `freeze`/`coding` inside** (a CP has no tests or code in slice-only mode; coding runs via `task-harness` per Task). **The coding chain MUST NOT be declared in the slice-only flow**: an `agent.delegate` node with no incoming forward edge is an entry node and is spawned at flow start (`flow_executor.go:97/152` spawns ALL `entryDelegateNodes`), and any incoming forward edge would auto-advance into coding — both break slice-only. Variant B (smoke) is a **separate opt-in flow file** `flows/cp-harness-smoke.yaml` (13 nodes incl. `audit`): same preflight + context + cp plan loop, then `task_splitter→freeze→test_signatures→implement→validate→reviewer→synthesis→audit→done` for the first sliced Task. `acceptance_nodes:[cp_synthesis, audit]` (smoke extends with `validate, synthesis`).
- `T-2` Add `apps/local-runner/internal/agentpack/flow-pack/prompts/plan-cp.md` — instructs to write `requirements/07-Coding-Plan/todo/CP-5x-*.md` per `FORMAT-REFERENCE-CP.md` §§1-10 with `DOD`, `P-*`, `R-*`, `Source Refs`, `Touched Areas`, and 3-Layer Defense.
- `T-3` Add `apps/local-runner/internal/agentpack/flow-pack/prompts/review-cp.md` — gate: `§10 DOD` measurable, `P-*` actionable+ordered, `§5 Touched Areas` exhaustive, `R-*` mitigations non-trivial, contradicts `CA` history check.
- `T-4` Add `apps/local-runner/internal/agentpack/flow-pack/prompts/task-splitter.md` — reads `CP.md` file_artifact (INPUT `cp_md`) via `file_artifact` resolver, splits each `P-*` into one `requirements/08-Task/todo/Task-*.md` per `FORMAT-REFERENCE-TASK.md` with `Parent Documents: CP-5x` + `P-*` traceability, `file_artifact` OUTPUT `task_md[]` (one per `P-*`, pathTemplate `Task-{{idx}}-{{slug}}.md`).
- `T-5` Update `apps/local-runner/internal/agentpack/flow-pack/manifest.yaml` — add `flows/cp-harness.yaml` + 3 prompts to `prompts`.
- `T-6` Add `apps/local-runner/internal/agentpack/cp_harness_pack_test.go` — `TestCpHarnessPackTopology` asserts **7 nodes**, plan loop = `preflight_contract_plan→context→cp_plan_writer→cp_reviewer→cp_synthesis` (no `freeze`/`test_signatures` anywhere in the slice-only flow; no `agent.code` node other than `cp_plan_writer` and `task_splitter`), `cp_plan_writer` `file_artifact` OUTPUT, `task_splitter` INPUT `cp_md` OUTPUT `task_md[]`, 1 `continue/back` (plan loop only), `acceptance_nodes:[cp_synthesis, audit]`, `cp-harness` selectable; `TestCpHarnessSmokePackTopology` asserts `cp-harness-smoke.yaml` = 13 nodes with the full coding chain and 2 `continue/back` with different `From`.
- `T-7` Do NOT seed `artifact_types`/`artifact_instances` here — instance seeding for `cp_md`/`task_md` is owned by Task-307 (service role, `is_builtin=true`); this task only declares the bindings in YAML (T-1) and asserts them in pack tests (T-6).

## Code Guide

### CG-1: `cp-harness.yaml` — Slice-Only Default (7 Nodes, NO freeze/test_signatures/coding)

```yaml
# apps/local-runner/internal/agentpack/flow-pack/flows/cp-harness.yaml
id: cp-harness
description: "Coding Plan (big feature / epic) — CP Plan Writer + CP Review Loop + Task Splitter (slice-only)"
builtin:
  editable: false
  selectableIn: [flow]
  cloneable: true
  mirror:
    required: true
    source: builtin
policy:
  cap: 3
  onCap: escalate
  extendBy: 2
  extendMax: 2
acceptanceNodes:
  - cp_synthesis
  - audit

nodes:
  # ─── Scout ───
  - id: preflight_contract_plan
    run: delegate
    lifecycle: once
    behavior: agent.delegate
    agent: agents/contract-planner.md          # Scout — Fast/Cheap model
    promptTemplate: prompts/plan-safe-fix-contract.md

  # ─── Draft Context (broad repo architecture excerpts) ───
  - id: context
    run: inline
    lifecycle: once
    behavior: context.produce

  # ─── CP Plan Phase (High-Reasoning model with 3-layer defense) ───
  - id: cp_plan_writer
    run: delegate
    lifecycle: reinvoke                        # reinvoke on cp_synthesis continue
    behavior: agent.code
    agent: agents/coder.md
    promptTemplate: prompts/plan-cp.md         # NEW prompt

  # NOTE: NO freeze/test_signatures here — a CP is planning only;
  # freeze + test_signatures + coding belongs to per-Task `task-harness` runs

  - id: cp_reviewer
    run: delegate
    lifecycle: spawn
    behavior: agent.delegate
    agent: agents/reviewer.md
    promptTemplate: prompts/review-cp.md       # NEW prompt
    dependsOn:
      - cp_plan_writer
    cohort: plan
    join: all

  - id: cp_synthesis
    run: inline
    lifecycle: reinvoke
    behavior: hub.inline
    agent: agents/synthesizer.md
    join: all

  # ─── Task Splitter ───
  - id: task_splitter
    run: delegate
    lifecycle: once
    behavior: agent.code                       # or context.produce if deterministic parse
    agent: agents/coder.md
    promptTemplate: prompts/task-splitter.md   # NEW prompt

  - id: audit
    run: inline
    lifecycle: once
    behavior: artifact.audit_draft

edges:
  # ── Scout → Draft Context → CP plan loop ──
  - { from: preflight_contract_plan, to: context, when: done, kind: forward }
  - { from: context, to: cp_plan_writer, when: done, kind: forward }
  - { from: cp_plan_writer, to: cp_reviewer, when: done, kind: forward }
  - { from: cp_reviewer, to: cp_synthesis, when: done, kind: forward }
  - { from: cp_synthesis, to: cp_plan_writer, when: continue, kind: back }   # LOOP back-edge

  # ── Plan approved → Slice → Audit (NO coding chain) ──
  - { from: cp_synthesis, to: task_splitter, when: done, kind: forward }
  - { from: task_splitter, to: audit, when: done, kind: forward }
  - { from: audit, to: done, when: done, kind: forward }
```

### CG-2: `cp-harness-smoke.yaml` — Opt-In 13 Nodes (First-Task Coding)

```yaml
# apps/local-runner/internal/agentpack/flow-pack/flows/cp-harness-smoke.yaml
id: cp-harness-smoke
description: "Coding Plan + first-Task smoke coding — CP Plan + Review + Split + TDD + Code Review"
builtin:
  editable: false
  selectableIn: []           # opt-in only, not in default /flow picker
  cloneable: true
  mirror:
    required: true
    source: builtin
policy:
  cap: 3
  onCap: escalate
  extendBy: 2
  extendMax: 2
acceptanceNodes:
  - cp_synthesis
  - validate
  - synthesis
  - audit

nodes:
  # ─── Identical to cp-harness preflight + CP plan loop ───
  # ... (copy nodes from cp-harness.yaml: preflight_contract_plan through cp_synthesis)
  # ... (copy task_splitter)

  # ─── Scope Lock & First-Task Coding Chain (like rag-harness code loop) ───
  - id: preflight_contract_freeze
    run: inline
    lifecycle: once
    behavior: contract.freeze

  - id: test_signatures
    run: delegate
    lifecycle: once
    behavior: agent.code
    agent: agents/tester.md
    promptTemplate: prompts/test-signatures.md

  - id: implement
    run: delegate
    lifecycle: reinvoke
    behavior: agent.code
    agent: agents/coder.md
    promptTemplate: prompts/implement-complete-tests.md

  - id: validate
    run: inline
    lifecycle: once
    behavior: command.validate

  - id: reviewer
    run: delegate
    lifecycle: spawn
    behavior: agent.delegate
    agent: agents/reviewer.md
    promptTemplate: prompts/review-safe-fix-contract.md
    dependsOn: [validate]
    cohort: review
    join: all

  - id: synthesis
    run: inline
    lifecycle: reinvoke
    behavior: hub.inline
    agent: agents/synthesizer.md
    join: all

  - id: audit
    run: inline
    lifecycle: once
    behavior: artifact.audit_draft

edges:
  # ── Same scout + CP plan loop edges as cp-harness ──
  # ...

  # ── Slice → Freeze Scope → First-Task coding chain ──
  - { from: task_splitter, to: preflight_contract_freeze, when: done, kind: forward }
  - { from: preflight_contract_freeze, to: test_signatures, when: done, kind: forward }
  - { from: test_signatures, to: implement, when: done, kind: forward }
  - { from: implement, to: validate, when: done, kind: forward }
  - { from: validate, to: implement, when: continue, kind: back }           # LOOP 2 back-edge
  - { from: validate, to: reviewer, when: done, kind: forward }
  - { from: validate, to: ask_user, when: escalate, kind: forward }
  - { from: reviewer, to: synthesis, when: done, kind: forward }
  - { from: synthesis, to: audit, when: done, kind: forward }
  - { from: synthesis, to: ask_user, when: escalate, kind: forward }
  - { from: audit, to: done, when: done, kind: forward }
```

### CG-3: `prompts/plan-cp.md` — CP Plan Writer Prompt Structure

```markdown
# CP Plan Writer — Coding Plan Architecture

You are an architecture plan writer. Your output MUST be a single markdown file
written to `requirements/07-Coding-Plan/todo/CP-{{cpID}}-{{slug}}.md`.

## Inputs Available
- `preflight_contract_plan` JSON (`candidate_feature_keys` + initial scope hypothesis from Scout)
- `FEATURE-KEYS.md` reference
- Latest `CA-*` change-audit history
- Tools: `read_file`, `grep_search` for targeted codebase investigation

## 3-Layer Defense: Architecture Verification
- Verify Scout's `candidate_feature_keys` and candidate scope against the architecture requirements.
- Use `read_file` / `grep_search` to investigate module boundaries, existing interfaces, and shared types.
- Ensure all sliced tasks in §4 have explicit `DeclaredPaths` and clear dependency ordering.

## Output Contract
Write exactly one `CP-*.md` per FORMAT-REFERENCE-CP.md (SS-13) with:
- §1 Goal
- §2 Input Documents (SD-*, SS-* references)
- §3 Implementation Strategy (approach, sequencing, dependencies)
  - §3.1 Topology Sketches (normative node/edge tables)
- §4 Work Breakdown (P-* items with file paths, line-level guidance)
- §5 Touched Areas (files, modules, database, external systems)
- §6 Data or Migration Steps
- §7 Validation Plan (tests to add, manual checks, failure cases)
- §8 Rollout and Fallback
- §9 Risks (R-* with mitigations)
- §10 Definition of Done (DOD-* checkboxes)

## Quality Gates
- Every P-* MUST be actionable with DeclaredPaths
- DOD-* items MUST be measurable (go test commands or manual steps)
- R-* mitigations MUST be non-trivial (not "be careful")
- §5 Touched Areas MUST be exhaustive (file paths, modules, tables)
```

### CG-4: `prompts/review-cp.md` — CP Reviewer Prompt Structure

```markdown
# CP Reviewer — Coding Plan Architecture Gate

You are reviewing a CP-*.md architecture document.

## Review Criteria (submit_review_outcome)
1. **§10 DOD Measurability**: Every DOD-* has a runnable verification
2. **P-* Actionability**: Each P-* has file paths + enough detail to implement
3. **§5 Touched Areas Exhaustive**: No missing file/module/table
4. **R-* Mitigations Non-Trivial**: Mitigations are specific, not generic
5. **CA Contradiction Check**: Changes don't contradict recent CA-* history
6. **P-* Ordering**: Sequencing logic in §3 is sound (dependencies honored)

## Outcome
- `approved` — all criteria clean
- `changes_requested` — findings per criterion (triggers cp_plan_writer re-entry)
```

### CG-5: `prompts/task-splitter.md` — Task Splitter Prompt Structure

```markdown
# Task Splitter — CP → Task Decomposition

You are splitting an approved CP-*.md into individual Task-*.md files.

## Input
- `CP.md` file_artifact (INPUT binding `cp_md`) — read via file_artifact resolver

## Output Contract
For each P-* in the CP's §4 Work Breakdown, write one:
  `requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md`

Each Task file MUST follow FORMAT-REFERENCE-TASK.md with:
- §Metadata: Parent Documents → link to this CP
- §AI Quick View: Summary referencing the P-* it implements
- §4 Exact Change: Copy the P-*'s file paths + guidance verbatim
- §6 Acceptance Check: Copy relevant DOD-* from the CP
- P-* traceability in Source Refs

## Constraints
- One Task per P-* (no merging, no splitting a single P-*)
- Task count N = number of P-* items in CP §4
- file_artifact OUTPUT binding: `task_md[]` (one per P-*)
```

### CG-6: Test Signatures

```go
// --- apps/local-runner/internal/agentpack/cp_harness_pack_test.go (NEW) ---

func TestCpHarnessPackTopology(t *testing.T) {
    pack, err := agentpack.LoadBuiltinPack()
    require.NoError(t, err)

    def := findFlowDef(t, pack, "cp-harness")

    // Assert: 7 nodes (slice-only default: scout, context, cp_plan_writer, cp_reviewer, cp_synthesis, task_splitter, audit)
    assert.Len(t, def.Nodes, 7)

    // Assert: plan loop = {preflight_contract_plan, context, cp_plan_writer, cp_reviewer, cp_synthesis}
    // NO test_signatures inside (CP has no tests yet)
    assertNodeExists(t, def, "preflight_contract_plan")
    assertNodeExists(t, def, "context")
    assertNodeExists(t, def, "cp_plan_writer")
    assertNodeExists(t, def, "cp_reviewer")
    assertNodeExists(t, def, "cp_synthesis")
    assertNodeNotExists(t, def, "test_signatures") // not in slice-only

    // Assert: forward edge chain
    assertEdgeExists(t, def.Edges, "preflight_contract_plan", "context", "done", "forward")
    assertEdgeExists(t, def.Edges, "context", "cp_plan_writer", "done", "forward")
    assertEdgeExists(t, def.Edges, "cp_plan_writer", "cp_reviewer", "done", "forward")
    assertEdgeExists(t, def.Edges, "cp_reviewer", "cp_synthesis", "done", "forward")
    assertEdgeExists(t, def.Edges, "cp_synthesis", "task_splitter", "done", "forward")
    assertEdgeExists(t, def.Edges, "task_splitter", "audit", "done", "forward")

    // Assert: no agent.code node other than cp_plan_writer and task_splitter
    agentCodeNodes := filterByBehavior(def.Nodes, "agent.code")
    codeIDs := map[string]bool{}
    for _, n := range agentCodeNodes {
        codeIDs[n.ID] = true
    }
    assert.True(t, codeIDs["cp_plan_writer"])
    assert.True(t, codeIDs["task_splitter"])
    assert.Len(t, agentCodeNodes, 2) // only these two

    // Assert: 1 continue/back (plan loop only — no code loop in slice-only)
    backEdges := filterBackEdges(def.Edges, "continue")
    assert.Len(t, backEdges, 1)
    assert.Equal(t, "cp_synthesis", backEdges[0].From)
    assert.Equal(t, "cp_plan_writer", backEdges[0].To)

    // Assert: acceptance_nodes = [cp_synthesis, audit]
    assert.Len(t, def.AcceptanceNodes, 2)
    assert.Contains(t, def.AcceptanceNodes, "cp_synthesis")
    assert.Contains(t, def.AcceptanceNodes, "audit")

    // Assert: cp_plan_writer prompt = plan-cp.md
    cpw := findNode(t, def, "cp_plan_writer")
    assert.Equal(t, "prompts/plan-cp.md", cpw.PromptTemplate)

    // Assert: cp_reviewer cohort=plan, join=all
    cpr := findNode(t, def, "cp_reviewer")
    assert.Equal(t, "plan", cpr.Cohort)
    assert.Equal(t, "all", cpr.Join)

    // Assert: task_splitter prompt = task-splitter.md
    ts := findNode(t, def, "task_splitter")
    assert.Equal(t, "prompts/task-splitter.md", ts.PromptTemplate)
}

func TestCpHarnessSmokePackTopology(t *testing.T) {
    pack, err := agentpack.LoadBuiltinPack()
    require.NoError(t, err)

    def := findFlowDef(t, pack, "cp-harness-smoke")

    // Assert: 13 nodes (CP plan loop + task_splitter + coding chain)
    assert.Len(t, def.Nodes, 13)

    // Assert: 2 continue/back edges with different From
    backEdges := filterBackEdges(def.Edges, "continue")
    assert.Len(t, backEdges, 2)
    froms := map[string]bool{}
    for _, e := range backEdges {
        froms[e.From] = true
    }
    assert.True(t, froms["cp_synthesis"]) // CP plan loop
    assert.True(t, froms["validate"])     // code loop

    // Assert: acceptance_nodes extends with validate + synthesis
    assert.Contains(t, def.AcceptanceNodes, "cp_synthesis")
    assert.Contains(t, def.AcceptanceNodes, "validate")
    assert.Contains(t, def.AcceptanceNodes, "synthesis")
    assert.Contains(t, def.AcceptanceNodes, "audit")

    // Assert: test_signatures exists in smoke variant
    assertNodeExists(t, def, "test_signatures")
    assertNodeExists(t, def, "implement")
}

// --- apps/local-runner/internal/runner/cp_harness_test.go (NEW) ---

func TestCpHarnessSliceOnlyTerminal(t *testing.T) {
    // Setup: cp-harness default flow (8 nodes)
    // Simulate: run through preflight → cp_plan_writer → cp_reviewer →
    //   cp_synthesis (done) → task_splitter → audit → done
    // Assert: step timeline contains NO implement/validate/reviewer/synthesis steps
    // Assert: no entry spawn beyond preflight_contract_plan
    // Assert: task_splitter writes file_artifact outputs
}

func TestCpHarnessSmokeVariant(t *testing.T) {
    // Setup: cp-harness-smoke flow (13 nodes)
    // Simulate: cp_synthesis continue → re-enters cp_plan_writer
    // Then: cp_synthesis done → task_splitter → test_signatures → implement →
    //   validate → reviewer → synthesis → audit → done
    // Assert: validate continue re-enters implement (code loop)
}
```

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

- result: implemented (commit 9dcf5541). cp-harness 7-node slice-only + cp-harness-smoke 13-node opt-in ship in the pack; entry spawn resolves to exactly [preflight_contract_plan]; plan loop live-proven (cp_synthesis continue reuses cp_plan_writer session).
- documented deviation: cp_plan_writer/task_splitter are agent.delegate (same CP-55 writer/freeze rationale as Task-305; slice-only has no freeze at all). task_splitter stays an LLM writer with prompts/task-splitter.md (Q-1's deterministic context.produce variant left as follow-up).
- tests added: TestCpHarnessPackTopology, TestCpHarnessSmokePackTopology, TestCpHarnessSelectableAndCloneable (cp_harness_pack_test.go); TestCpHarnessEntrySpawnIsScoutOnly, TestCpHarnessPlanLoopContinueReusesCpPlanWriter (cp_harness_test.go).
- follow-ups: live /flow cp-harness slice round + artifact panel check of cp_md/task_md (DOD-5 manual); EnsureBuiltinFlowMirrorsWithStore sync against a live Supabase.
- upstream docs updated: `CP-58` `P-4`, §3.1
