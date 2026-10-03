# CA-1149 — BUG-618: resume-from-node could not spawn `agent.scaffold` targets

Live trigger: BUG-616 rewind of run-150388. After restoring sprint index 2,
the parked "Resume from context" gate consumed `ok`, flipped the loop to
running, resolved `context → tdd` — then logged
`flow_advance_target_not_spawnable` and spawned nothing. The vibe-sprint
`tdd` node's behavior is `agent.scaffold`, but `tryAdvanceFlowFromNode`'s
spawnable allowlist only listed `agent.delegate | agent.code |
agent.reproduce`. The scaffold family was already accepted by the
audit-dispatch path (`IsScaffoldBehavior`) — only the executor's advance path
lagged.

Symptom class: an accepted gate decision that silently does nothing is the
worst resume failure — the run reads `running` while no leg exists. This
also explains earlier mid-flow wedges where a scaffold re-drive after a
gate/debate appeared to no-op.

## Fix

`tryAdvanceFlowFromNode` now gates spawnability on
`agentpack.ProviderBackedBehavior(canonical)` — the canonical delegate-scope
family table (`agent.delegate | agent.code | agent.reproduce |
agent.scaffold`) that stays in sync with `behavior_registry.go`'s
scope-delegate registrations, instead of a hand-maintained triple.

## Tests (additive)

- `TestTryAdvanceFlowFromNodeSpawnsScaffold` — red before fix on all three
  providers: `context → tdd` advance returns false and no leg spawns. Green
  after: scaffold-architect leg spawns, joined note reaches the hub, the
  synthesis reprompt dispatches (parity verified per provider).

## Files

- `apps/local-runner/internal/runner/flow_executor.go`
- `apps/local-runner/internal/runner/bug618_scaffold_advance_spawnable_test.go` (new)
