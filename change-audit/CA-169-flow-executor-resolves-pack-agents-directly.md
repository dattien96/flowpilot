# CA-169: Flow Executor Resolves Pack Agents Directly, Immune to Catalog Shadowing (BUG-NOTE-CP42 #23)

## Scope

Verified and fixed a P1 issue from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`: a built-in flow (Review Loop, RAG Harness) could silently run the wrong agent definition if a project happened to define its own agent file with a matching name.

## The bug

`review-loop.yaml` declares `agent: agents/coder.md`; `flowNodeAgentName` reduces this to the bare catalog name `"coder"`, which `spawnChildRun` then resolves through `AgentCatalog.listAgents(cwd)` — whose documented precedence is project-local (`.claude/agents`, `.codex/agents`) **>** provider-home **>** built-in. Pack agents (`loadBuiltinAgentDefinitionsFromPack`, feeding `builtinAgentDefinitions()`) are merged in at the lowest tier alongside this same catalog. So a repo with its own `.claude/agents/coder.md` — defined for a completely unrelated purpose, with no intent to affect the flow pack at all — would silently replace the built-in Review Loop's coder agent's system prompt, tools, and role. This directly contradicted CP-42/CA-158's "pack-driven, not hardcoded" design: the flow's own YAML declares exactly which agent file it wants, but the runtime never actually enforced that declaration once it collapsed to a bare name.

## Fix

Added `SpawnAgentInput.AgentDefOverride *AgentDefinition` (`agent_orchestrator.go`) — an internal-only field (`json:"-"`, same pattern as the existing `UIInitiated`) that bypasses `spawnChildRun`'s catalog lookup entirely when set. Added `resolvePackAgentDefinition(agentName string) (*AgentDefinition, bool)` (`flow_executor.go`), which looks up the embedded pack's own bundled agent definitions directly via `loadBuiltinAgentDefinitionsFromPack()` — the same loader the catalog itself uses for its lowest tier, but consulted here without any project-local/provider-home layer in front of it.

All three places `flow_executor.go` spawns a delegate node now resolve and pass this override:
- `startResolvedFlow`'s entry-node spawn loop
- `startInlineEntryChain`'s single delegate spawn (CA-165)
- `tryAdvanceFlowFromNode`'s reviewer-cohort spawn loop (CA-163)

`spawnChildRun` (`interactive_service.go`) checks `in.AgentDefOverride` first, falling back to the normal name-based catalog resolution only when it's nil. A `resolvePackAgentDefinition` miss (pack unreadable, or no agent by that name — shouldn't happen for either built-in flow today) also falls back to the catalog path rather than failing the spawn outright, matching this file's existing safe-bail convention.

**Scope note**: this only changes flow-executor-initiated spawns. The AI-driven `spawn_agent` tool (used for freeform, non-flow-pack agent orchestration) never sets `AgentDefOverride`, so a project's own custom agents remain fully reachable by name through the normal catalog precedence for that path — this fix narrows the pack-driven flow-executor path specifically, without touching the general-purpose catalog's documented precedence.

## Verification

- New test `TestStartResolvedFlowSpawnsPackAgentEvenWhenProjectShadowsItsName` (`flow_executor_test.go`): writes a `.claude/agents/coder.md` shadow agent (`role: hijacked`) into a temp workspace, starts `review-loop` via `startResolvedFlow`, and asserts the spawned coder child's `role` is still the pack's own `"coder"`, not `"hijacked"`.
- Full flow-related test group (68 tests) passes unchanged.
- Full suite: 997 passed, 15 pre-existing/environmental failures (Windows paths, missing local `codex` CLI, fixture assumptions — none in a file touched by this change).
- `go build ./...` clean.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: add SpawnAgentInput.AgentDefOverride and resolvePackAgentDefinition so flow-executor-initiated spawns always run the pack's own bundled agent, immune to a same-named project-local or provider-home agent shadowing it via the general catalog's precedence
# --->8---
