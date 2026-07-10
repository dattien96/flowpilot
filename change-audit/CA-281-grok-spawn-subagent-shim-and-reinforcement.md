# CA-281: Grok Spawn Subagent Shim And Spawn Reinforcement

## Scope

Task-209 DOD-8 follow-up: Grok models prefer native `spawn_subagent` over FlowPilot MCP `spawn_agent`, so children never reached the agent panel. Adds prompt reinforcement (primary steer) plus a native-tool notification shim (fallback) and UI tool-name normalization.

## Root Cause

- Live Grok turns exposed native `spawn_subagent` in the initialize tool list; the model answered it uses that tool, not FlowPilot `spawn_agent`.
- `grokAskUserReinforcement` existed (CA-278) but there was no spawn equivalent; registry `promptPrep` only appended ask-user guidance.
- Native `spawn_subagent` runs inside `grok agent` and never calls `TurnBridge.SpawnAgent`.

## Changes

- `apps/local-runner/internal/runner/grok_adapter.go`
  - `grokSpawnAgentReinforcement`, `grokToolReinforcements`, `tryShimGrokNativeSpawnSubagent`, `parseGrokSpawnSubagentInput`, `grokSubagentTypeToFlowPilotAgent`.
- `apps/local-runner/internal/runner/grok_event_mapper.go`
  - `grokRawToolName`, `grokNormalizedToolDisplayName` — alias `spawn_subagent`→`spawn_agent`, `ask_user_question`→`ask_user` at event/UI boundary (GR-07).
- `apps/local-runner/internal/runner/provider_registry.go`
  - Live Grok `promptPrep` appends `grokToolReinforcements`.
- Tests: `grok_mcp_test.go`.

## Verification

- `go test ./internal/runner -run 'Grok.*(Spawn|Subagent|Mcp|Prompt|Native|Normalized|ParseGrok)'` — PASS.
- Live desktop DOD-8 retest: pending user confirmation that Grok uses MCP `spawn_agent` or shim-visible children after reinforcement.

## Follow-ups

- Task-209 DOD-4/5/7/9 remain open.
- Parent hang after UI spawn while main `running` is a separate status-persistence issue.

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: Task-209
change_type: feature
summary: Grok spawn_agent reinforcement steers off native spawn_subagent; notification shim mirrors native spawn_subagent to TurnBridge.SpawnAgent with UI tool-name normalization
# --->8---