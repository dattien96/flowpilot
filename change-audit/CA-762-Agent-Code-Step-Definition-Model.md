# CA-762 — Non-inline steps honor Settings step_definitions.model

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-320
change_type: bugfix
summary: agent.code writers (implement, test_signatures) honor Settings step_definitions.model; empty inherits the run model
# --->8---

## Change

Operator rule: every non-inline step can set a model on Desktop Settings > Workflows > Step Definitions; the value lives in `step_definitions.model`; non-empty wins inherit.

- `resolveFlowNodeModel`: spawnable behaviors are `agent.delegate` **and** `agent.code`. Inline/control still return `""`.
- `spawnFrozenWriterChild`: passes `delegateSpawnModel` into `SpawnAgentInput.Model` and stamps step posture (freeze-chain writers previously always inherited the parent run).
- Desktop `stepDefinitionRequiresModel` / `flowStepShowsModel` / `FLOW_BEHAVIOR_OPTIONS`: `agent.code` is a real agent (field shown). Empty model option is `(inherit)`; save persists `null` instead of blocking "Model is required."
- Pack YAML `model:` stays Task-320 delegate-only (no builtin pins). Runtime reads the admin row, not YAML, for writers.

## Why this shape

`implement` / `test_signatures` already spawn child runs (`BehaviorScopeDelegate`, same handler as `agent.delegate`). Task-320 T-3 treated `agent.code` as inherit-only so YAML `model:` would not be silently ignored. Settings rows existed via mirror but the form hid Model and spawn never passed it.

## Tests

- New `agent_code_step_model_test.go`: implement row wins (claude/codex/grok model ids), test_signatures row wins, empty inherit, inline `command.validate` ignores a row, `spawnFrozenWriterChild` child uses gpt-5.4-mini not parent claude-sonnet, empty row inherits parent.
- New desktop tests: `stepModelVisibility.agent-code.test.ts`, `FlowStepTimeline.agent-code-model.test.ts`.
- Old: `TestResolveFlowNodeModel*`, `TestFlowNodeModelValidation`, BUG-290 visibility tests untouched and green.

## Provider impact

**Case 1 agnostic.** `resolveFlowNodeModel` / `spawnFrozenWriterChild` take no `providerKey` and never branch on one. Matrix is model-id prefixes (claude-/gpt-/grok-) so a future provider split trips the same tests.

## Prior CA intact

- CA-728 / Task-320: YAML `model:` still fail-closed on non-delegate pack nodes.
- BUG-290: inline steps still hide model in Settings and timeline.
- CA-616: planner still skips DB rows.
- BUG-228: `SpawnAgentInput.Model` still wins inherit for delegate; now also for frozen writers.

## Residual (reviewer CA-762)

- Settings path uses `resolveConfiguredModelForAgent` (catalog `step_definitions` rows). Builtin + cloned flows that keep those rows work.
- `recordFromWorkflowRow` still stamps `node.Model` only for `agent.delegate` (BUG-352 / pack validator). Widening that to `agent.code` trips `TestFlowNodeModelValidation` (old test; not edited). Comment on `resolveFlowNodeModel` no longer claims DB-carry for writers.
- Flow-Mode picker runs leave `chatFlowRef` empty (`flowRefForRun`), so catalog pass 1 (flow-scoped) never fires. Dual-harness `implement`/`test_signatures` with models on both rows: pass 2 `len(hits)!=1` → inherit. Pre-existing CA-358; not blocking.

