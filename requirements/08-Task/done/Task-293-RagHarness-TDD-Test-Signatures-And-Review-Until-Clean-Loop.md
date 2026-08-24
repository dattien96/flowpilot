# Task-293: Rag-Harness TDD Test-Signatures Step And Review-Until-Clean Loop

## Metadata

- Document ID: `Task-293`
- Title: `Rag-Harness TDD Test-Signatures Step And Review-Until-Clean Loop`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-08-25`
- Last Updated: `2026-08-25`
- Parent Documents: `CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance`
- Child Documents: `None`
- Related Documents: `SS-15-Agent-Review-Loop-Until-Clean`, `CP-53-Review-Loop-Done-Requires-Machine-Verdict`, `.agents/skills/flow-mode-orchestrator/SKILL.md`, `.agents/skills/safe-fix-contract/SKILL.md`
- Replaces: `N/A`
- Tags: `rag-harness, agent-flow-engine, tdd, test-signatures, review-loop, safe-fix-contract`

## AI Quick View

### Summary

- Upgrades the built-in `rag-harness` flow from `plan → freeze → context → implement → validate → audit` to the full Flow Mode lifecycle: `plan → freeze → context → test_signatures → implement → validate → reviewer → synthesis → audit`.
- Adds a dedicated **test-signatures** writer step (empty unit-test signatures only, no logic), an **implement** step that fills those signatures plus the production code, and a **review-until-clean** gate (single reviewer cohort + hub synthesis) that loops back to implement on findings.
- Wires the previously-decorative `promptTemplate` node field into real prompt composition so plan/review/test/implement nodes carry their own static instructions (including the safe-fix-contract rules on plan and review).

### Current Ask

- Make the built-in rag-harness follow the flow-mode-orchestrator lifecycle: unit-test signatures written before implementation, validation covering both old and new tests, and a review loop that sends findings straight back to the coder until the review gate approves.

### Key Decisions

- `T-1` `test_signatures` is a real `agent.code` writer (tester agent) governed by the same frozen preflight contract as `implement` — the freeze node binds the contract to every `agent.code` writer in the flow.
- `T-2` The review gate is a single `cohort: review` reviewer + `hub.inline` synthesis (cheaper than the two-reviewer review-loop; SS-15 allows the degenerate single-reviewer shape). `acceptance_nodes` includes `synthesis` so the CP-53 machine-verdict gate applies.
- `T-3` The flow declares exactly ONE `continue` back-edge (`validate → implement`); synthesis's `changes_requested` resolves the same edge via `resolveContinueBackEdgeTarget`, so review findings re-enter at implement while validate retries reuse the identical edge.
- `T-4` `FlowNode.PromptTemplate` becomes a real static prompt source: `composeFlowNodeAgentPrompt` appends the node's declared template, skipping Go render templates (contents containing `{{`).

### Constraints

- Do not modify `review-loop.yaml` or `context-coding-review-synthesis.yaml`.
- Do not modify the shared agent files (`contract-planner.md`, `coder.md`, `tester.md`, `reviewer.md`, `synthesizer.md`).
- Pre-existing green tests stay untouched except the explicitly approved minimal adapter change in `TestFlowCodingPromptSpawnWrappedDoesNotDuplicateHistory`.
- The planner's JSON contract (`feature_key`/`intent`/`declared_paths`/`source_doc_id`) is unchanged — no new planner fields.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/agentpack/flow-pack/flows/rag-harness.yaml`
- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go`
- `apps/local-runner/internal/runner/flow_executor.go`
- `apps/local-runner/internal/runner/artifact_type_registry.go`
- `apps/local-runner/internal/changecontract/preflight.go`

## 1. Goal

Extend the built-in rag-harness flow with a TDD test-signatures step before implementation and a bounded review-until-clean loop after validation, following the flow-mode-orchestrator lifecycle, while keeping the existing plan-freeze-context backbone and the CP-55 frozen-contract guarantees.

## 2. Parent Links

- coding plan: `requirements/07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md`
- system spec: `requirements/05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md`
- skill: `.agents/skills/flow-mode-orchestrator/SKILL.md`, `.agents/skills/safe-fix-contract/SKILL.md`

## 3. Trigger

rag-harness is the Flow Mode default (chatBaseline) but stops after validate/audit: no test-first step and no adversarial review loop, so findings never loop back to the coder automatically. The flow-mode-orchestrator skill (plan → freeze → context → test-signatures → implement → validate → review-loop → audit) should be emulated by the real engine.

## 4. Exact Change

### 4.1 Flow topology (`rag-harness.yaml`)

New nodes:

| Node | Behavior | Agent | Lifecycle | Notes |
|---|---|---|---|---|
| `test_signatures` | `agent.code` | `tester` | `once` | empty signatures only; `promptTemplate: prompts/test-signatures.md` |
| `reviewer` | `agent.delegate` | `reviewer` | `spawn` | `cohort: review`, `join: all`, `promptTemplate: prompts/review-safe-fix-contract.md` |
| `synthesis` | `hub.inline` | `synthesizer` | `reinvoke` | `join: all`; `tools: [submit-review-outcome]` |

Changed nodes: `implement.promptTemplate` → `prompts/implement-complete-tests.md`; `preflight_contract_plan.promptTemplate` → `prompts/plan-safe-fix-contract.md`.

`acceptance_nodes: [validate, synthesis, audit]`.

Edges: `context → test_signatures → implement → validate`; `validate --done--> reviewer → synthesis`; `validate --continue--> implement` (the single continue back-edge); `synthesis --done--> audit → done`; `synthesis --escalate--> ask_user`; `validate --escalate--> ask_user`.

### 4.2 Static node prompts

Four new static markdown prompts (no Go template braces), declared in `manifest.yaml`:

- `plan-safe-fix-contract.md` — safe-fix rules (feature-key + CA history, additive tests, 3-provider, matrix coverage); response stays a single JSON object.
- `test-signatures.md` — signatures only, no production code, no bodies.
- `implement-complete-tests.md` — fill signatures, safe-fix short form.
- `review-safe-fix-contract.md` — R1/R2/R3/history gate; approve only when clean.

### 4.3 Engine wiring

- `composeFlowNodeAgentPrompt` (`artifact_type_registry.go`): appends `node.PromptTemplate` when it names a loadable static template; skips templates containing `{{` (render-only, e.g. `flow-context-handoff.md`).
- `runContractFreezeNode` (`flow_validate_audit_dispatch.go`): after freezing the first writer, binds the same draft to every other `agent.code` writer (`bindFrozenContractToSiblingWriters`), including on the reuse/recovery path; reload-verifies every bound writer.
- `advanceFlowThroughFreezeChain`: spawn block extracted into `spawnFrozenWriterChild` (shared with `tryAdvanceFlowFromNode`).
- `tryAdvanceFlowFromNode` (`flow_executor.go`): forward-done targets may now be `agent.code`; such targets spawn through `spawnFrozenWriterChild` with their bound contract (blocking spawn when the contract is missing) — never the review handoff.
- `runValidateNode` passed case: routes through `tryAdvanceFlowFromNode` (so `validate → reviewer` cohort spawn works), preserving the terminal (`done`/`ask_user`) settle for legacy validate edges.
- `behaviors/registry.yaml`: added the `agent.code` behavior doc entry; `rag-harness#implement` moved out of `agent.delegate.usedBy`; reviewer/synthesis added to their usedBy lists.

## 5. Tests

### 5.1 Updated (inventory / approved minimal adapter)

- `agentpack/pack_test.go` `TestLoadBuiltinRAGHarnessFlow`: node count 6 → 9; asserts the new nodes, promptTemplates, tools, and acceptance nodes.
- `runner/flow_context_handoff_test.go` `TestFlowCodingPromptSpawnWrappedDoesNotDuplicateHistory`: adapter captures the implement child's prompt via the `[FlowPilot implement step]` marker instead of the raw intent substring (the tester's prompt now also embeds the intent; double-close would panic); workspace git-initialized so the frozen-writer gate observation succeeds. All original assertions unchanged.

### 5.2 New (additive)

- `agentpack/rag_harness_tdd_review_pack_test.go` — topology, edges, safety (both writers freeze-dominated + acceptance), manifest prompt declarations.
- `runner/compose_static_prompt_template_test.go` — static append, render-template skip, missing-template no-op, prompt hard-rule contents.
- `runner/flow_freeze_multi_writer_bind_test.go` — freeze binds test_signatures + implement with distinct ContractIDs; reuse binds missing sibling; writer-node classifier.
- `runner/flow_advance_agent_code_writer_test.go` — tester completion spawns implement via the writer path (implement prompt, never review handoff); blocks unbound writer spawn.
- `runner/rag_harness_validate_to_reviewer_test.go` — validate passed spawns the single cohort reviewer (with cohort id), join drives synthesis RUNNING; legacy validate→audit inline chain preserved.
- `runner/rag_harness_synthesis_edges_test.go` — synthesis continue reinvokes implement with findings (session reuse); synthesis approved dispatches audit.

## 6. DoD

- All new tests green; full `agentpack` + `changecontract` suites green; `runner` suite has zero new failures vs baseline (7 pre-existing environment failures verified identical with `git stash`).
- Pre-existing tests untouched except the two documented in §5.1 (inventory + approved adapter fix).
- Cross-provider parity: all engine changes are provider-agnostic (no `ProviderKey` branching introduced).