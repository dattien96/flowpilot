# CA-158: Generic Flow Executor — Entry-Node Spawn From A Resolved flowRef

## Scope

Make a resolved `flowRef` selection actually run something. Previously (CA-152/154/156), `subMode`/`flowRef` were validated and sent from the UI but had zero execution effect. This closes that gap for the entry point only, without touching the live cohort/hub-reinvoke machinery those flows already run on.

## Design (why this is safe)

Investigated how Review Loop actually starts execution today: it does **not** start automatically. An AI hub agent, following its own skill/system-prompt guidance, decides to call the `spawn_agent` tool with `autoOrchestrate: true`. That tool call is handled by `spawnChildRun` (`interactive_service.go`), which:
- creates the child run,
- stamps `parent.autoOrchestrate = true` and the loop's `Mode = "explicit"`,
- lets the existing cohort/join/`isCoderRun`/`maybeAutoReinvokeHub`/`flow_control` machinery drive everything after that.

`spawnChildRun` is already decoupled from HTTP/tool-argument parsing — it takes a typed `SpawnAgentInput` and works identically regardless of caller. This meant the entry point CP-42 needed was already exposed: call `spawnChildRun` directly, with `SpawnAgentInput` built from the resolved `FlowDefinition`'s entry node, instead of waiting on the AI to decide to call the tool itself. **No cohort/join/cap/transition logic needed to be built or duplicated.**

## Completed

- `TurnInput.SubMode`/`.FlowRef` (previously validated in `handleStartTurn` then silently dropped — an oversight from the earlier CA-154 pass) are now threaded through into the struct `startTurn` receives.
- `startTurn`'s existing `if rs.turnCount == 0 { ... }` block (already the exact "first turn only" gate the desktop's `changeType`/`sourceDocId` logic uses) now also checks `in.FlowRef`, and if set, kicks off `go s.startResolvedFlow(...)` — scheduled async specifically because `spawnChildRun` manages its own `s.mu` locking and must not run while `startTurn` still holds the lock.
- `flow_executor.go`: `startResolvedFlow` resolves the flowRef via `FlowDefinitionResolver` (nil-store-safe: falls back to the embedded pack, which is correct for a built-in flow), finds the flow's **entry nodes** (`entryDelegateNodes`: nodes with no `dependsOn` and `behavior` normalizing to `agent.delegate`), derives each entry node's catalog agent name from its `agent:` file path (`flowNodeAgentName`, e.g. `agents/coder.md` → `coder`, matching `agentpack.parseAgentSpec`'s own default-name derivation), and spawns each via `spawnChildRun` with `AutoOrchestrate: true`. For `review-loop.yaml` this correctly identifies exactly one entry node (`coder`) — `reviewer_correctness`/`reviewer_security` are excluded via `dependsOn: [coder]`, and `synthesis` is excluded via its `hub.inline` behavior.
- `InteractiveService.flowDefinitionStore` (new, optional field) + `SetFlowDefinitionStore` setter, wired from `cmd/flowpilot runner serve` using the same `FlowDefinitionStoreFor` resolution CA-157 added for mirror sync — one store instance now backs both mirror sync and flowRef execution.
- A spawn or resolve failure is logged only; the run's own first turn to its own provider proceeds unaffected either way.

## Explicitly not touched

- The hub's own first-turn prompt/behavior is unchanged — this only guarantees the coder spawn happens deterministically; it does not change what the parent (hub) run itself says or does on its own first turn. Whether the hub's own conversational framing should change when a flow is Go-initiated (e.g. to avoid the hub redundantly also trying to do the coding) is a real product/UX question this session did not attempt to answer, since guessing at it risks a confusing double-execution and there was no existing pattern to follow.
- `isCoderRun`/cohort/join/`flow_control` handling in `interactive_service.go` — completely unmodified. The entry spawn is the only new code in the live execution path.

## Verification

- `TestEntryDelegateNodesReturnsOnlyNoDependencyDelegateNodes` / `...EmptyWhenNoMatch`, `TestFlowNodeAgentNameDerivesFromFilePath` — pure unit tests.
- `TestStartResolvedFlowSpawnsOnlyTheEntryNode` — integration-level: resolves `flowpilot-core-flow-pack/review-loop` against a real `InteractiveService`, asserts exactly one `coder` child is spawned (never a reviewer/synthesis child directly), and that `loopMode` becomes `"explicit"`.
- `TestStartResolvedFlowUnknownFlowRefSpawnsNothing` — failure path spawns nothing, no panic.
- `TestStartTurnWithFlowRefSpawnsEntryNodeAsynchronously` — proves the actual wiring (`startTurn` → async `startResolvedFlow`), not just the underlying function: a real `startTurn` call carrying `SubMode`/`FlowRef` results in a coder child appearing within 2s, without the turn call itself blocking.
- Full suite: no new failures beyond the known pre-existing/flaky set (`TestProjectRunHistoryFiltersRunsByProject` intermittently, confirmed unrelated in earlier sessions).

## Follow-ups

- Deciding whether/how the hub's own first-turn framing should change when Go-initiated (vs. AI-initiated) is open.
- This only starts the entry node; RAG Harness (`rag-harness.yaml`) has a different topology (context-producer entry, not `agent.delegate`) and was not exercised by this change — `entryDelegateNodes` would currently find no entry node for it, which is correct (RAG Harness is a Flow Mode template, not something Chat Mode's `flowRef` picker offers per `chatBaseline: true`).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-177
change_type: feature
summary: spawn a resolved flowRef's entry node via the existing spawnChildRun path, making Chat Mode flow selection actually execute
# --->8---
