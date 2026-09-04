# CA-728 — per-node model tiering for harness delegate nodes (Task-320)

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-320
change_type: feature
summary: pack YAML agent.delegate nodes gain optional model field with fail-closed validation against agentpack.ModelProviderKey prefix table; runner resolves DB step row (non-planner admin override) over YAML node.Model over inherit, planner honors YAML only with DB rows skipped (CA-616); mirror upsert never writes model and recordFromWorkflowRow carries it for cloned flows
# --->8---

## Problem

- CP-58 P-7 / Task-305 follow-up "per-node model tiering not yet wired":
  pack YAML had no `model:` field, so Scout-cheap vs Architect
  high-reasoning tiering lived only in per-installation DB edits.
- Runner `providerKeyFromModel` prefix table was duplicated inline, and a
  typo'd model would silently inherit instead of failing at pack load.
- Planner (`contract-planner` / `preflight_contract_plan`) had a hardcoded
  `""` with a CA-616 guard against the legacy `gpt-5.4` seeded row; there
  was no pack-controlled way to tier it.

## Changes

- `apps/local-runner/internal/agentpack/pack.go`: `FlowNode.Model` +
  `flowNodeFromMap` `model:` parse (trimmed); new `ModelProviderKey`
  canonical prefix table (`gpt-`→`codex`, `gemini-`/`auto-gemini-`→`gemini`,
  `claude-`→`claude`, `grok-`/`grok-build`→`grok`,
  `opencode/`/`opencode-go/`→`opencode`); `ValidateFlowDefinition`
  fail-closed (model on non-delegate rejected, unknown-prefix delegate
  model rejected).
- `apps/local-runner/internal/runner/provider_registry.go`:
  `providerKeyFromModel` delegates to `agentpack.ModelProviderKey`
  (byte-identical behavior, parity-tested).
- `apps/local-runner/internal/runner/flow_executor.go`:
  `resolveFlowNodeModel` planner branch returns `node.Model`, other
  delegates return DB-row hit else `node.Model` else `""`;
  `delegateSpawnModel` simplifies to `resolveFlowNodeModel` (CA-616
  rationale moved onto the row-skip).
- `apps/local-runner/internal/runner/supabase_workflow_flow_store.go`:
  `workflowSelect` selects `model`, `dbStepDefinitionRow.Model` carried in
  `recordFromWorkflowRow` onto `node.Model` (planner included); mirror
  `upsertNodeStepDefinitions` unchanged (never writes `model`, so re-sync
  cannot clobber admin values — BUG-249 principle).
- Tests (additive only): `node_model_pack_test.go`
  (`TestModelProviderKeyPrefixes`, `TestFlowNodeModelValidation`);
  `node_model_resolution_test.go`
  (`TestProviderKeyFromModelMatchesPackTable`,
  `TestResolveFlowNodeModelFallsBackToYamlPackDefault`,
  `TestResolveFlowNodeModelDbRowBeatsYamlPackDefault`,
  `TestResolveFlowNodeModelPlannerHonorsYamlIgnoresRow`,
  `TestRecordFromWorkflowRowCarriesNodeModel`).
- Docs: Task-305 follow-up line flipped to wired-by-Task-320; CP-58 P-7
  mechanism note (no builtin pins — provider-agnostic constraint).

## Verification

- `go test ./internal/agentpack/ -count=1` PASS (incl. new tier tests).
- `go test ./internal/runner/ -run
  'TestResolveFlowNodeModel|TestProviderKeyFromModel|TestRecordFromWorkflowRow'
  -count=1` PASS (planner-YAML-honored, CA-616 legacy-row-ignored,
  parity, DB-carry).
- `go test ./internal/runner/ -run
  'TestRAGHarnessLive|TestReviewLoop|TestContextCoding' -count=1` PASS.
- Full `go test ./internal/runner/` shows only pre-existing environmental
  failures (verified identical on stashed baseline:
  `TestResumeRunEchoesChatIdentity`,
  `TestRunCompatCheckIncludesPortabilityCanaries`,
  `TestEnsureGitNexusIndex_AutoAnalyzesOnce`,
  `TestResolveGoogleDriveMcpProviderStatuses_AllNotStarted`,
  `TestSpawnChildEmitsGraphAndBusEvents`,
  `TestFlowCodingPromptSpawnWrappedDoesNotDuplicateHistory` flake;
  `TestTryAdvanceSpawnsAgentCodeWriterWithWriterPrompt` passes in
  isolation) — zero new regressions from this change.
