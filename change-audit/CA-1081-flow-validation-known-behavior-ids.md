# CA-1081: flow validation knows builtin-only behavior ids

Date: 2026-10-01
Refs: user-reported warning on the workflow steps settings page —
`Step "flowpilot_core_flow_pack_vibe_sprint_preflight_contract_freeze" has an
unrecognized Behavior ID "contract.freeze"` and the same for
`flowpilot_core_flow_pack_vibe_sprint_tdd` ("agent.scaffold").

## Root cause

`validateFlowGraph` treated `FLOW_BEHAVIOR_OPTIONS` — the **user-authoring
picker list** — as the runner's complete dispatch table. Mirrored builtin
`step_definitions` legitimately carry behavior ids the runner dispatches but
the picker never offers (`agentpack.behaviorAliases` canonical targets not in
the picker): `contract.freeze`, `agent.scaffold` (vibe-sprint),
`agent.reproduce` (bug-harness), `tournament.arbiter`, `tournament.merge`.
Every builtin-mirrored flow graph therefore produced false "unrecognized
Behavior ID" warnings.

## Change

`packages/flowpilot-client-core/src/domain/adminModels.ts`:

- New `BUILTIN_ONLY_BEHAVIOR_IDS` — the 5 non-selectable canonical ids with
  `requiresAgent` mirroring the runner's provider-backed set
  (`agent.scaffold`/`agent.reproduce` = true; `contract.freeze`/`tournament.*`
  = false).
- New `KNOWN_BEHAVIOR_OPTIONS = FLOW_BEHAVIOR_OPTIONS ∪
  BUILTIN_ONLY_BEHAVIOR_IDS` — the full 17-id runner-dispatchable set (verified
  1:1 against `agentpack/pack.go` `behaviorAliases` canonical values).
- `validateFlowGraph` derives `knownBehaviorIds` and
  `behaviorsRequiringAgent` from `KNOWN_BEHAVIOR_OPTIONS`.

`apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`:

- `stepDefinitionAgentRefIssue` (the inline row warn) reads
  `KNOWN_BEHAVIOR_OPTIONS` so a builtin `agent.scaffold` step missing
  `agentRef` still surfaces the missing-agent warning.
- The Behavior ID authoring picker unchanged — still `FLOW_BEHAVIOR_OPTIONS`;
  builtin-only ids stay hidden from user authoring.

## Files

- `packages/flowpilot-client-core/src/domain/adminModels.ts`
- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`
- `apps/desktop-flowpilot/src/components/settings/ca1081_validateFlowGraph_builtin_behaviors.test.ts`

## Verification

- `tsc --noEmit` clean.
- New test (3 cases): builtin ids accepted; builtin provider behavior without
  agentRef still warns; unknown `foo.bar` still fails.
- `npm run test:phase1`: 719 pass / 12 fail — identical pre-existing
  baseline set (jira helpers, history-replay, runnerRepositories, style
  tokens); no new failures.
- Behavior ids validated against `behaviorAliases` canonical map —
  12 selectable + 5 builtin-only = all 17 runner-dispatchable ids covered.

## Out of scope

`contract.freeze`'s runner handler is still the CP-55 P-3 fail-closed
placeholder (`not implemented yet`) — this change fixes the false *validator*
warning only; implementing the freeze handler is a separate task.
