# CA-119: Provider-Consistent Agent Spawn Prompt Composition

## Scope

- Unify the spawned sub-agent first-turn prompt across providers (BUG-128)
- Name the agent and link its definition file in the composed prompt

## Completed

- Added `composeAgentSpawnPrompt(agentDef, userPrompt)` and `composeAgentIdentityLine(def)` in `apps/local-runner/internal/runner/interactive_service.go`.
- Routed the `spawnChildRun` composition through `composeAgentSpawnPrompt` (replacing the inline `agentDef.SystemPrompt + "\n\n" + in.Prompt`), so every provider produces the same prompt shape: system prompt first, then a single `[FlowPilot sub-agent — agent: … | role: … | definition: <path|built-in (<source>)>]` line, then the user prompt.
- Kept the agent system prompt first so `hasBuiltInAgentPromptPrefix` / `isAgentHistoryRun` (backend) and its frontend twin keep detecting built-in agent runs.
- Added unit test `TestComposeAgentSpawnPromptIsProviderConsistent`.
- Updated SD-16 §7.3 item 2 with the provider-independent composition rule.

## GitNexus Impact

- GitNexus MCP tools were not connected in this thread and the index is flagged stale; impact assessed by local inspection per repo fallback policy.
- `spawnChildRun`: the only composition consumer is the child's first turn (and `pendingTurnPrompt` for dependency-blocked starts); the change is additive (identity line) and order-preserving (system prompt stays first).
- Downstream prompt-prefix detection (`hasBuiltInAgentPromptPrefix`, `navigatorHistory.ts`) verified unaffected because the system prompt remains the prompt's prefix.
- No HIGH or CRITICAL impact: composition is internal to the single spawn path shared by the AI tool and UI spawn.

## Verification

- `TestComposeAgentSpawnPromptIsProviderConsistent` — pass.
- `TestUISpawnInjectsContextIntoParentProviderTurn`, `TestToolSpawnWaitTrueDoesNotInjectParentContext`, `TestToolSpawnWaitFalseInjectsResult` — pass.
- No-regression: `go test ./internal/runner/ -run 'Agent|Spawn|Catalog|History|Resume|Codex' -count=1` failure set is identical with and without the change (10 pre-existing environment failures: real-`codex`-binary resume, compat/cross-account, provider-home skill merges); the change adds one passing test and zero new failures.
- `go build ./internal/runner/...` — pass.

## Residual Notes

- Cross-provider content differences for the same agent name remain a function of catalog precedence (BUG-125), not the composition path; this change only unifies and labels the shape.
- Built-in agents have no on-disk path, so their identity line shows `definition: built-in (<source>)` rather than a file link.

# ---8<--- flowpilot:change-ledger
feature_key: agent-spawn
source_doc_id: BUG-128
change_type: feature
summary: Provider-Consistent Agent Spawn Prompt Composition
# --->8---
