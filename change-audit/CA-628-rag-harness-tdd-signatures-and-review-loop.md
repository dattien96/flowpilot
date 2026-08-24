# CA-628: Rag-harness TDD test-signatures step and review-until-clean loop

## What

Upgraded the built-in `rag-harness` flow (Flow Mode chatBaseline) from `plan → freeze → context → implement → validate → audit` to the full flow-mode-orchestrator lifecycle: `plan → freeze → context → test_signatures → implement → validate → reviewer → synthesis → audit`, with a single continue back-edge (`validate → implement`) so both validate retries AND review findings re-enter at implement. Plan and review steps now carry the safe-fix-contract rules via real static node prompts.

## Why

rag-harness stopped after validate/audit: no test-first step, no adversarial review loop, and the `FlowNode.PromptTemplate` field was decorative (parsed/persisted but never composed into prompts). The flow-mode-orchestrator skill's lifecycle (test-signatures before implementation, review-loop that bounces findings straight back to the coder until clean) had no engine equivalent.

## Fix

- **`flow-pack/flows/rag-harness.yaml`**: added `test_signatures` (agent.code / tester / once), `reviewer` (agent.delegate / cohort review / join all), `synthesis` (hub.inline / join all); `acceptance_nodes: [validate, synthesis, audit]`; `tools: [submit-review-outcome]`; `promptTemplate` on plan/implement/test_signatures/reviewer nodes; edges `context → test_signatures → implement → validate`, `validate --done--> reviewer → synthesis`, `synthesis --done--> audit → done`. Exactly ONE `continue` back-edge (`validate → implement`) — the pack validator rejects duplicate back-edges for the same status, and synthesis's `changes_requested` resolves the same edge via `resolveContinueBackEdgeTarget`, so review findings re-enter at implement (reset covering implement → validate → reviewer → synthesis) while validate's own retry path reuses the identical edge.
- **`flow-pack/prompts/`**: 4 new static prompts — `plan-safe-fix-contract.md` (feature-key + CA history, additive tests, 3-provider, matrix coverage; response stays one JSON object), `test-signatures.md` (signatures only), `implement-complete-tests.md` (fill signatures; safe-fix short form), `review-safe-fix-contract.md` (R1/R2/R3 + history gate). Declared in `manifest.yaml`.
- **`runner/artifact_type_registry.go`** `composeFlowNodeAgentPrompt`: appends `node.PromptTemplate` when it names a loadable static template; skips templates containing `{{` (render-only Go templates like `flow-context-handoff.md` must never be injected raw).
- **`runner/flow_validate_audit_dispatch.go`**:
  - `runContractFreezeNode`: binds the same frozen draft to EVERY `agent.code` writer (`bindFrozenContractToSiblingWriters`, idempotent, incl. reuse/recovery path), and reload-verifies every bound writer — so `implement`'s gate finds its own contract even though `test_signatures` is the freeze chain's first writer.
  - `advanceFlowThroughFreezeChain`: spawn block extracted into shared `spawnFrozenWriterChild` (rendered FCP + static node prompt + change contract; logs `flow_contract_freeze_writer_spawned`).
  - `runValidateNode` passed case: routes through `tryAdvanceFlowFromNode` so `validate --done--> reviewer` spawns the cohort member; terminal (`done`/`ask_user`) targets still settle via `advanceToNextInlineOrDelegate`.
- **`runner/flow_executor.go`** `tryAdvanceFlowFromNode`: forward-done targets may now be `agent.code`; they spawn via `spawnFrozenWriterChild` with their bound contract — never the review handoff — and spawn is blocked (diag only) when the frozen contract is missing.
- **`flow-pack/behaviors/registry.yaml`**: added `agent.code` doc entry; `rag-harness#implement` moved out of `agent.delegate.usedBy`; `rag-harness#reviewer`/`#synthesis` added to their usedBy lists.

## Tests (additive; two documented pre-existing edits)

New: `agentpack/rag_harness_tdd_review_pack_test.go`, `runner/compose_static_prompt_template_test.go`, `runner/flow_freeze_multi_writer_bind_test.go`, `runner/flow_advance_agent_code_writer_test.go`, `runner/rag_harness_validate_to_reviewer_test.go`, `runner/rag_harness_synthesis_edges_test.go` (12 new tests, 3-provider-neutral).

Pre-existing edits (operator-approved, documented in Task-293 §5.1):
- `agentpack/pack_test.go` `TestLoadBuiltinRAGHarnessFlow` — node-count inventory 6 → 9 (+ new-node assertions). Inventory guard, not a behavior oracle.
- `runner/flow_context_handoff_test.go` `TestFlowCodingPromptSpawnWrappedDoesNotDuplicateHistory` — adapter now captures the implement child's prompt via the `[FlowPilot implement step]` static marker (the tester's prompt also embeds the frozen intent; the old substring capture would double-close the channel and panic), and the fixture workspace is git-initialized so the frozen-writer gate observation succeeds. All original assertions unchanged.

Old: `go test ./internal/agentpack ./internal/changecontract` green; `go test ./internal/runner` has zero NEW failures vs baseline (7 pre-existing environment failures — firebase MCP JSON, supabase shaping, /work paths, TempDir cleanup, canonical-batch, gate-settle, frozen-context — verified identical with `git stash`); `go vet ./internal/runner ./internal/agentpack ./internal/changecontract` clean.

Provider parity: **Case 1 agnostic** — the changed symbols (`composeFlowNodeAgentPrompt`, `runContractFreezeNode`, `tryAdvanceFlowFromNode`, `runValidateNode`, `spawnFrozenWriterChild`) take no `ProviderKey` and add no provider branching; the review gate reuses the existing cohort/hub machinery already exercised by review-loop across Claude/Codex/Grok.

Will not undo: CP-55 P-8 migration topology (plan/freeze/context backbone, agent.code writers), CA-587 hub-less audit WAITING fallback (audit node still exists with the same behavior), CA-616/617 delegate-fail park semantics, CA-623 freeze-escalate stamping, review-loop.yaml / context-coding-review-synthesis.yaml untouched.

Residual risk (disclosed): live rag-harness now has a hub.inline (synthesis), so planner-fail live behavior follows the CA-355 hub-reinvoke path rather than the CA-616 hub-less park path; hub-less fixtures (`ragHarnessNodes()`, `flowFixtureEdgesNodes()`, TUI F2) are hand-built and were NOT changed. An empty Go test suite is green for `command.validate` — empty/stubbed test bodies are caught by the review gate and the implement prompt, not by validate.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-293
change_type: feature
summary: rag-harness gains TDD test-signatures writer + single-reviewer review-until-clean loop; static promptTemplate wiring; freeze binds every agent.code writer (Task-293)
# --->8---