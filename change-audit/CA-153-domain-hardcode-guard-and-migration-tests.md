# CA-153: Domain Hardcode Guard And Migration Tests

## Scope

Implement the achievable slice of `Task-180`: a regression guard against new semantic hardcodes, plus migration tests proving legacy step types and unrelated custom step types both behave correctly on the current active path.

## Completed

- Added `TestDomainHardcodeGuardMatchesFrozenBaseline` (`domain_hardcode_guard_test.go`): scans `interactive_service.go`, `agent_catalog.go`, `review_loop_config.go`, `flow_context_handoff.go`, and `agent_orchestrator.go` for quoted string-literal occurrences of the role/step-name words CP-42 flags (`reviewer`, `coder`, `synthesizer`, `plan`, `planning`, `design`, `coding`, `implementation`, `code`, `review-loop`), and asserts the count per file matches an audited, documented baseline. This is a frozen-inventory guard, not a claim that all hardcodes are gone: it fails if a *new* one appears anywhere in these files (or an existing one is silently removed) without the baseline and this note being updated together.
- Audited baseline: `flow_context_handoff.go` and `agent_orchestrator.go` are clean (0 hits — CA-147/CA-151 already routed step classification through `agentpack.NormalizeBehaviorID`). `agent_catalog.go` (6) and `review_loop_config.go` (7) carry their documented legacy Go fallback literals (Task-174 T-3 / Task-175-176 fallback). `interactive_service.go` (4) carries the still-outstanding `isAgentRole(rs, "coder")` branches — the one exception this guard does **not** consider resolved.
- Added `flow_pack_migration_test.go`:
  - `TestArbitraryNodeStepTypeIsInertWithoutRunnerChange` — a step type unrelated to any pack alias (`"totally-custom-node-xyz"`) is not classified as Plan or Coding and leaves `injectFlowContextIfCoding`'s prompt untouched, demonstrating a genuinely new node name needs a pack alias entry, not a runner code change.
  - `TestLegacyPlanCodingStepTypesStillResolveThroughAliasTable` — the old CP-41 literal step types (`plan`, `planning`, `design`, `coding`, `implementation`, `code`) still resolve correctly through the alias table.

## Explicitly not attempted here (and why)

- **Removing the four `isAgentRole(rs, "coder")` branches in `interactive_service.go`.** These sit inside the live cohort/hub-reinvoke orchestration logic. Migrating them onto behavior dispatch is a real behavioral change to the run-loop, not an additive wrapper like the CA-151 RAG-harness rewire — it needs full call-graph visibility to be safe. GitNexus MCP tools (mandated by this repo's `CLAUDE.md` for pre-edit impact analysis) were not connected in this session, so this was deliberately deferred rather than edited blind.
- **CP-42's "existing sessions resume" / "existing built-in flows still pass integration tests" acceptance items** are already covered by the pre-existing `TestE2EReviewLoop*` suite, which was re-run and still passes unchanged; no new test was needed there.
- A pre-existing, order-dependent flaky test (`TestProjectRunHistoryFiltersRunsByProject`, unrelated to any file touched this session) was observed failing intermittently under `-count=3`; not investigated as out of scope for CP-42.

## Verification

- `go test ./internal/runner -run 'TestDomainHardcodeGuard|TestArbitraryNodeStepType|TestLegacyPlanCodingStepTypes'`
- `go test ./internal/runner/... ./internal/agentpack/...` (full suite)

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-180
change_type: refactor
summary: add a frozen domain-hardcode inventory guard and migration tests for legacy and unrelated step types
# --->8---
