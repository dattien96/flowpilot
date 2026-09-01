# Task-305: Task-Harness 11-Step With Plan Writer And Plan Review Loop

## Metadata

- Document ID: `Task-305`
- Title: `Task-Harness 11-Step With Plan Writer And Plan Review Loop`
- Phase: `task`
- Status: `inprogress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-27`
- Last Updated: `2026-09-01`
- Parent Documents: [CP-58: Bug / Task / CP Harness With Plan Artifact And Dual Review Loops](../../07-Coding-Plan/todo/CP-58-Bug-Task-Cp-Harness-Plan-Review-Loop.md)
- Child Documents: `None`
- Related Documents: [CP-55: Flow-First Preflight Contract](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md), [Task-293: Rag-Harness TDD + Review Loop](../../08-Task/done/Task-293-RagHarness-TDD-Test-Signatures-And-Review-Until-Clean-Loop.md), [rag-harness.yaml](../../../apps/local-runner/internal/agentpack/flow-pack/flows/rag-harness.yaml), [Task-304: Dual Back-Edge](../todo/Task-304-Dual-Back-Edge-Validator-And-Edge-Driven-Re-entry.md)
- Replaces: `None`
- Tags: `flow, harness, task, plan-artifact, review-loop, agent-flow-engine`

## AI Quick View

### Summary

- Adds built-in `task-harness.yaml` 12-node: `plan (Scout)→context (Draft)→plan_writer (Architect)→plan_reviewer→plan_synthesis --continue-->plan_writer --done-->freeze (Lock Scope)→test_signatures→implement→validate→reviewer→synthesis→audit`.
- `context.produce` provides draft code excerpts to `plan_writer`; `plan_writer` has 3-layer defense (override `feature_key` if Scout misjudges + grep/read tool fallback) and writes `Task-*.md` as `file_artifact`; `plan_reviewer` + `plan_synthesis` gate the plan and feature-key; only AFTER plan approved does `freeze` lock scope for coding.
- Keeps `rag-harness` byte-identical as `bug-harness` fallback; `task-harness` is `selectableIn:flow cloneable:true mirror:required`.

### Current Ask

- Ship the Tier-2 harness so a normal `Task` (1-3d, needs HLD/LLD) gets a reviewed plan artifact before `implement`.

### Key Decisions

- `T-1` `plan_writer` is `agent.code` (not `agent.delegate`) with `read_file`/`grep_search` fallback tools and `feature_key` override capability; `ValidateFlowSafetyTopology` binds `contract.freeze` and enforces `acceptance_nodes` inclusive of `plan_synthesis`.
- `T-2` `context.produce` runs before `plan_writer` to provide draft code excerpts; `contract.freeze` runs AFTER `plan_synthesis` approved to lock the final scope.
- `T-3` `bug-harness` kept as alias/clone of current `rag-harness` — no edge/prompt change — so Bug scope pays zero extra turns.
- `T-4` Two `hub.inline` nodes (`plan_synthesis` + `synthesis`) require Task-304's `activeHubNodeID` tracking; do not rely on `hubInlineNodeID` first-match for step marking in this flow.
- `T-5` **Model Tiering & 3-Layer Defense:** `preflight_contract_plan` (Scout) runs with a Fast/Cheap model tier producing `candidate_feature_keys` and `candidate_paths`; `plan_writer` and `plan_reviewer` run with the **Highest Reasoning model** (e.g. Claude 3.7 Sonnet, o3, Grok 4.5 high) to author and gate the `Task-*.md` HLD/LLD plan artifact with feature-key verification.

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

- `T-1` Add `apps/local-runner/internal/agentpack/flow-pack/flows/task-harness.yaml` — 12 nodes incl. `audit` (11 steps + terminal), edges: `plan→context→plan_writer→plan_reviewer→plan_synthesis --continue(back)--> plan_writer --done(forward)--> freeze→test_signatures→implement→validate --done--> reviewer→synthesis --done--> audit→done`, `validate --continue(back)--> implement`. `acceptance_nodes:[plan_synthesis, validate, synthesis, audit]`, `mode:flow`, `builtin: {editable:false, selectableIn:[flow], cloneable:true, mirror:{required:true, source:builtin}}`, `policy:{cap:3, onCap:escalate, extendBy:2, extendMax:2}`.
- `T-2` Add `apps/local-runner/internal/agentpack/flow-pack/prompts/plan-task.md` — instructs to read `preflight_contract_plan` intent + `context` package + last `CA` history, write exactly one `requirements/08-Task/todo/Task-xxx-*.md` per `SS-13` + `FORMAT-REFERENCE-TASK.md` §1-§8 with `Metadata`, `AI Quick View`, `Goal`, `Parent Links`, `Trigger`, `Exact Change`, `Touched Areas`, `Acceptance Check`, `Out of Scope`, `Completion Notes`; include `P-*` traceability, `DeclaredPaths`, `Risks`, `Source Refs`. Enforce `file_artifact` OUTPUT path via `declared_paths`. (Task files have no `DOD` section per format — the acceptance gate lives in `Acceptance Check`.)
- `T-3` Add `apps/local-runner/internal/agentpack/flow-pack/prompts/review-plan.md` — gate: `AC` completeness, `P-*` traceability, single happy-path matrix (needs `R3` near-miss, mirroring `review-safe-fix-contract.md`), `DeclaredPaths` coverage, contradicts `CA` history; `test_signatures` not empty/stubbed; approve via `submit_review_outcome` only when all clean (record-only per `CP-53`).
- `T-4` Update `apps/local-runner/internal/agentpack/flow-pack/manifest.yaml` — add `flows/task-harness.yaml` to `flows`, add `prompts/plan-task.md` + `prompts/review-plan.md` to `prompts` (keep `cloneable/editable` sync per `pack.go:769`).
- `T-5` Optional `flows/bug-harness.yaml` as byte-identical clone of `rag-harness.yaml` with `id: bug-harness` and label, or keep `rag-harness` and alias — whichever avoids duplicating drift.
- `T-6` Add `apps/local-runner/internal/agentpack/task_harness_pack_test.go` — `TestTaskHarnessPackTopology` asserts 12 nodes, 2 `continue/back` with different `From`, `acceptance_nodes` includes `plan_synthesis`, `plan_writer` promptTemplate = `prompts/plan-task.md`, `plan_reviewer` `cohort:plan join:all`, exactly 2 `hub.inline` nodes (`plan_synthesis`, `synthesis`), and no `test_signatures` inside the plan loop (it sits between `plan_writer` and `plan_reviewer`, exactly once).

## Code Guide

### CG-1: `task-harness.yaml` — Full YAML Skeleton (`flows/task-harness.yaml`)

```yaml
# apps/local-runner/internal/agentpack/flow-pack/flows/task-harness.yaml
id: task-harness
description: "Task (1-3d feature slice) — Plan Writer + Plan Review Loop + TDD + Code Review Loop"
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
  - plan_synthesis
  - validate
  - synthesis
  - audit

nodes:
  # ─── Scout ───
  - id: preflight_contract_plan
    run: delegate
    lifecycle: once
    behavior: agent.delegate
    agent: agents/contract-planner.md          # Scout — Fast/Cheap model
    promptTemplate: prompts/plan-safe-fix-contract.md

  # ─── Draft Context (provides code excerpts for plan_writer) ───
  - id: context
    run: inline
    lifecycle: once
    behavior: context.produce

  # ─── Plan Phase (High-Reasoning model with 3-layer defense) ───
  - id: plan_writer
    run: delegate
    lifecycle: reinvoke                        # reinvoke on plan_synthesis continue
    behavior: agent.code
    agent: agents/coder.md                     # or agents/planner.md if Q-1 resolved
    promptTemplate: prompts/plan-task.md       # NEW prompt

  - id: plan_reviewer
    run: delegate
    lifecycle: spawn
    behavior: agent.delegate
    agent: agents/reviewer.md
    promptTemplate: prompts/review-plan.md     # NEW prompt (checks feature_key + doc contract)
    dependsOn:
      - plan_writer
    cohort: plan
    join: all

  - id: plan_synthesis
    run: inline
    lifecycle: reinvoke
    behavior: hub.inline
    agent: agents/synthesizer.md
    join: all

  # ─── Scope Lock (runs ONLY AFTER plan approved) ───
  - id: preflight_contract_freeze
    run: inline
    lifecycle: once
    behavior: contract.freeze

  # ─── Code Phase (identical to rag-harness) ───
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
    dependsOn:
      - validate
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
  # ── Scout → Draft Context → Plan loop ──
  - { from: preflight_contract_plan, to: context, when: done, kind: forward }
  - { from: context, to: plan_writer, when: done, kind: forward }
  - { from: plan_writer, to: plan_reviewer, when: done, kind: forward }
  - { from: plan_reviewer, to: plan_synthesis, when: done, kind: forward }
  - { from: plan_synthesis, to: plan_writer, when: continue, kind: back }          # LOOP 1 back-edge

  # ── Plan approved → Freeze scope → Code ──
  - { from: plan_synthesis, to: preflight_contract_freeze, when: done, kind: forward }
  - { from: preflight_contract_freeze, to: test_signatures, when: done, kind: forward }
  - { from: test_signatures, to: implement, when: done, kind: forward }

  # ── Code loop (identical edges to rag-harness) ──
  - { from: implement, to: validate, when: done, kind: forward }
  - { from: validate, to: implement, when: continue, kind: back }                 # LOOP 2 back-edge
  - { from: validate, to: reviewer, when: done, kind: forward }
  - { from: validate, to: ask_user, when: escalate, kind: forward }
  - { from: reviewer, to: synthesis, when: done, kind: forward }
  - { from: synthesis, to: audit, when: done, kind: forward }
  - { from: synthesis, to: ask_user, when: escalate, kind: forward }
  - { from: audit, to: done, when: done, kind: forward }
```

### CG-2: `prompts/plan-task.md` — Plan Writer Prompt Structure

```markdown
# Plan Writer — Task HLD/LLD

You are a technical plan writer. Your output MUST be a single markdown file
written to `requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md`.

## Inputs Available
- `preflight_contract_plan` JSON (`candidate_feature_keys` + initial scope hypothesis from Scout)
- `context` package (draft source.excerpt, change.contract from context.produce)
- `FEATURE-KEYS.md` reference
- Latest `CA-*` change-audit history
- Tools: `read_file`, `grep_search` for targeted codebase investigation

## 3-Layer Defense: Feature-Key Override Contract
- Review Scout's `candidate_feature_keys` against the user request and `FEATURE-KEYS.md`.
- If Scout misjudged the feature key (e.g. assigned a terminal paste bug to `chat-engine`), you MUST override it with the correct feature key in `§Metadata` of `Task-*.md`.
- Use `read_file` / `grep_search` if the draft context is missing specific structs or functions.

## Output Contract
Write exactly one `Task-*.md` per FORMAT-REFERENCE-TASK.md (SS-13 §5.1):
- §Metadata: Document ID, Parent Documents (link to CP if applicable), Tags, Verified Feature Key
- §AI Quick View: Summary, Current Ask, Key Decisions (T-*), Constraints, Open Questions (Q-*), Source Refs
- §1 Goal
- §2 Parent Links (coding plan, tech design, system spec, specific upstream ids)
- §3 Trigger
- §4 Exact Change (T-* items with file paths and line-level guidance)
- §5 Touched Areas (files, modules, routes, tables)
- §6 Acceptance Check (go test commands, manual checks)
- §7 Out of Scope
- §8 Completion Notes

## Quality Gates
- Every T-* MUST have a `DeclaredPaths` file reference
- Acceptance Check MUST have runnable `go test` commands
- P-* traceability to parent CP (if any)
- Source Refs with line numbers
- Risks section if >2 files touched
```

### CG-3: `prompts/review-plan.md` — Plan Reviewer Prompt Structure

```markdown
# Plan Reviewer — Task/CP Plan Gate

You are reviewing a plan artifact (Task-*.md or CP-*.md) and its test signatures.

## Review Criteria (submit_review_outcome)
1. **Feature-Key & Scope Contract**: `feature_key` in §Metadata correctly maps to domain logic and exists in `FEATURE-KEYS.md`
2. **AC Completeness**: §6 Acceptance Check has runnable commands, not just prose
3. **P-* Traceability**: Every T-* traces to a parent P-* (if CP exists)
4. **DeclaredPaths Coverage**: Every T-* in §4 has explicit file paths
5. **Single Happy-Path Matrix**: At least one failure/edge case addressed (R3 analog)
6. **CA Contradiction Check**: Changes don't contradict recent CA-* history
7. **Test Signatures**: test_signatures output is non-empty and covers declared paths
8. **SS-13 Contract**: All required sections present per FORMAT-REFERENCE-TASK.md

## Outcome
- `approved` — all criteria clean (unblocks `preflight_contract_freeze`)
- `changes_requested` — list specific findings per criterion (triggers plan_writer re-entry)
```

### CG-4: `manifest.yaml` Changes

```yaml
# Add to flows: section
flows:
  # ... existing entries ...
  - flows/task-harness.yaml

# Add to prompts: section (if manifest tracks prompts)
prompts:
  # ... existing entries ...
  - prompts/plan-task.md
  - prompts/review-plan.md
```

### CG-5: `bug-harness.yaml` — Clone of `rag-harness.yaml`

```yaml
# flows/bug-harness.yaml — byte-identical to rag-harness.yaml except id/description
id: bug-harness
description: "Bug / Hotfix (single file, clear AC) — TDD + Code Review Loop"
# ... rest identical to rag-harness.yaml nodes/edges/policy ...
```

### CG-6: Test Signatures

```go
// --- apps/local-runner/internal/agentpack/task_harness_pack_test.go (NEW) ---

func TestTaskHarnessPackTopology(t *testing.T) {
    // Load builtin pack
    pack, err := agentpack.LoadBuiltinPack()
    require.NoError(t, err)

    // Find task-harness flow definition
    def := findFlowDef(t, pack, "task-harness")

    // Assert: 12 nodes (11 steps + audit terminal)
    assert.Len(t, def.Nodes, 12)

    // Assert: 2 continue/back edges with DIFFERENT From
    backEdges := filterBackEdges(def.Edges, "continue")
    assert.Len(t, backEdges, 2)
    froms := map[string]bool{}
    for _, e := range backEdges {
        froms[e.From] = true
    }
    assert.True(t, froms["plan_synthesis"])  // plan loop
    assert.True(t, froms["validate"])        // code loop

    // Assert: acceptance_nodes includes plan_synthesis
    assert.Contains(t, def.AcceptanceNodes, "plan_synthesis")
    assert.Contains(t, def.AcceptanceNodes, "validate")
    assert.Contains(t, def.AcceptanceNodes, "synthesis")
    assert.Contains(t, def.AcceptanceNodes, "audit")

    // Assert: plan_writer uses plan-task.md prompt
    pw := findNode(t, def, "plan_writer")
    assert.Equal(t, "prompts/plan-task.md", pw.PromptTemplate)

    // Assert: plan_reviewer cohort=plan, join=all
    pr := findNode(t, def, "plan_reviewer")
    assert.Equal(t, "plan", pr.Cohort)
    assert.Equal(t, "all", pr.Join)

    // Assert: exactly 2 hub.inline nodes
    hubNodes := filterByBehavior(def.Nodes, "hub.inline")
    assert.Len(t, hubNodes, 2)
    hubIDs := map[string]bool{}
    for _, n := range hubNodes {
        hubIDs[n.ID] = true
    }
    assert.True(t, hubIDs["plan_synthesis"])
    assert.True(t, hubIDs["synthesis"])

    // Assert: plan loop connects scout -> context (draft) -> plan_writer -> plan_reviewer -> plan_synthesis
    assertEdgeExists(t, def.Edges, "preflight_contract_plan", "context", "done", "forward")
    assertEdgeExists(t, def.Edges, "context", "plan_writer", "done", "forward")
    assertEdgeExists(t, def.Edges, "plan_writer", "plan_reviewer", "done", "forward")
    assertEdgeExists(t, def.Edges, "plan_reviewer", "plan_synthesis", "done", "forward")

    // Assert: plan approved flows into freeze (scope lock) -> test_signatures -> implement
    assertEdgeExists(t, def.Edges, "plan_synthesis", "preflight_contract_freeze", "done", "forward")
    assertEdgeExists(t, def.Edges, "preflight_contract_freeze", "test_signatures", "done", "forward")
    assertEdgeExists(t, def.Edges, "test_signatures", "implement", "done", "forward")
}

func TestTaskHarnessValidateFlowDefinition(t *testing.T) {
    // Load task-harness, call ValidateFlowDefinition
    // Assert: no error (dual back-edge passes with Task-304 fix)
}

// --- apps/local-runner/internal/runner/task_harness_dual_loop_test.go (NEW) ---

func TestTaskHarnessDualLoopReinvoke(t *testing.T) {
    // Setup: task-harness flow with 12 nodes
    // Simulate: plan_synthesis emits continue
    //   → plan_writer session reused (reinvoke)
    //   → plan_reviewer + plan_synthesis reset to PENDING
    //   → preflight_contract_plan and context stay DONE (not forward-reachable from plan_writer)
    // Then: plan_synthesis emits done → freeze → test_signatures → implement
    // Simulate: synthesis emits continue
    //   → implement session reused (reinvoke)
    //   → validate + reviewer + synthesis reset to PENDING
    //   → plan-loop nodes, context, and freeze stay DONE (not forward-reachable from implement)
}
```

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

- result: implemented (commit e54801fa). 12-node task-harness loads through full ValidateFlowDefinition with two anchored continue/back loops; plan loop live-proven in fake-provider runner tests: plan_synthesis continue reuses the plan_writer session, code hub continue routes validate->implement and never enters the plan loop.
- documented deviation: plan_writer is agent.delegate + reinvoke, NOT agent.code as T-1 sketched: CP-55 ValidateFlowSafetyTopology requires every agent.code writer dominated by contract.freeze, and plan_writer must run BEFORE freeze. agent.code shares the delegate handler verbatim and artifact write contracts key on artifactBindings, so runtime semantics are unchanged while real code writers (test_signatures, implement) stay agent.code under the CP-55 contract.
- tests added: TestTaskHarnessPackTopology, TestTaskHarnessValidateFlowDefinition (task_harness_pack_test.go); TestTaskHarnessPlanLoopContinueReusesPlanWriter, TestTaskHarnessCodeLoopContinueStaysOutOfPlanLoop (task_harness_dual_loop_test.go).
- follow-ups: live plan-review reject->revise->approve->freeze->code round (DOD-4 manual); per-node model tiering (Scout cheap vs Architect high-reasoning) is prompt/model config, not yet wired.
- upstream docs updated: `CP-58` `P-2`, `P-3`, §3.1
