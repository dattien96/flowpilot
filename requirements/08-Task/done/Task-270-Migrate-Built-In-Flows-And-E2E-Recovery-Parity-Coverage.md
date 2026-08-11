# Task-270: Migrate Built-In Flows And Add End-to-End Recovery/Parity Coverage (CP-55 P-8)

## Metadata

- Document ID: `Task-270`
- Title: `Migrate Built-In Flows And Add End-to-End Recovery/Parity Coverage`
- Phase: `task`
- Status: `done` (2026-07-31 — implemented, then adversarially reviewed by a dedicated Claude reviewer agent: 1 Critical, 3 Important, 2 Minor findings — a FAIL verdict, the most severe of any CP-55 phase reviewed so far. The Critical finding was a real, concretely-demonstrated bypass of CP-55's central guarantee: a coder's ordinary file-write tool could forge an arbitrary Canonical Head for any feature by appending a raw NDJSON line to the pending-canonical store's exempted bookkeeping path, which `finalizePendingCanonicalHeadsForRun` then trusted byte-for-byte with zero provenance check. All 4 Critical/Important findings fixed and re-verified via mutation testing (each temporarily reverted, the corresponding test confirmed to fail, then restored to pass) — including an HMAC-signature scheme (`PendingCanonicalRecord.Signature`, reusing the existing `markerSecret` trust-boundary infrastructure) that closes the forgery gap without changing behavior for any pre-existing test. Both Minor findings accepted/documented, not fixed. Full `internal/runner` regression re-run after fixes: exactly the established 15 pre-existing failures plus 1 already-documented load-dependent flake, 0 new deterministic regressions. See [CA-431](../../../change-audit/CA-431-migrate-built-in-flows-and-e2e-recovery-parity-coverage.md) for the complete findings/fix accounting.)
- Owner: `FlowPilot Architecture`
- Reviewers: `Claude (agent review, complete — see CA-431)`
- Created: `2026-07-31`
- Last Updated: `2026-07-31`
- Feature Keys: `context-regression-engine`, `agent-flow-engine`
- Parent Documents: [CP-55: Flow-First Preflight Contract, Context Retrieval, and Canonical Acceptance](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md) (P-8), [Task-266](./Task-266-Enforce-Frozen-Scope-At-Coder-Gate-And-Amendments.md) (P-4 — the `agent.code` gate this task closes a real gap in), [Task-267](./Task-267-Move-Canonical-Mutation-To-Terminal-Flow-Acceptance.md) (P-5 — `PendingCanonicalStore`, whose own bookkeeping-path exemption gap this task closes), [Task-269](./Task-269-Wire-Ranking-Into-Feature-History.md) (P-7 — `buildRetrievalLocus`/ranked `feature.history`, first wired to a real Flow by this task)
- Child Documents: `none`
- Related Documents: [CA-426](../../../change-audit/CA-426-contract-planner-freeze-node-and-inline-chain-advancement.md), [CA-427](../../../change-audit/CA-427-enforce-frozen-scope-at-coder-gate-and-amendments.md), [CA-428](../../../change-audit/CA-428-move-canonical-mutation-to-terminal-flow-acceptance.md)
- Replaces: `None`
- Tags: `agent-flow-engine, context-regression-engine, preflight-contract, built-in-flows, e2e, migration`

## AI Quick View

### Summary

Migrates the three real, production built-in Flow YAMLs (`review-loop`, `rag-harness`, `context-coding-review-synthesis`) to the CP-55 target pattern (`preflight-contract-plan -> preflight-contract-freeze -> [context.produce, where the flow already had one] -> coder: agent.code -> existing acceptance chain`), and proves the full mechanism end to end: restart/recovery safety, cross-provider parity, and zero Normal-chat regression. Implementation surfaced and fixed three genuine, previously-undetected production bugs (an `agent.code` writer's gate pass had zero Canonical Head effect at all; a writer's own pending-canonical staging write self-triggered scope drift on its next gate pass; the built `FlowContextPackage` was never actually rendered into the writer's prompt when the freeze node has no intermediate `context.produce` hop — review-loop's own shape) — each found through this effort's established mutation-testing/full-regression discipline, not by inspection alone.

### Current Ask

Implement P-8 exactly: migrate all three built-in flows with concrete `acceptance_nodes`, verify Claude/Codex/Grok parity, add restart/recovery tests across freeze/gate/finalization, and drive the full 18-test signature list from the coding plan to green with zero new regressions against the established baseline.

### Key Decisions

- `D-1` **The `agent.code` writer's gate-pass gap is fixed via a new parallel path sourced from the `FrozenContractRecord` directly, not the legacy `Contract` parser.** Consistent with P-4's own design decision (Task-266/CA-427: a second, independent lifecycle rather than extending the legacy `Contract` type) — `isCodingChild` is now also set true when `frozenOK && len(drift) == 0`, with a synthetic `preparedChangeContract` built straight from the frozen record's own fields.
- `D-2` **The pending-canonical-store bookkeeping-path exemption mirrors `FrozenStore`'s own exact-file-match idiom, not a directory prefix.** A directory-wide exemption was exactly the security hole CA-427 Finding 2 closed for `FrozenStore`'s own bookkeeping files; the new `IsPendingCanonicalStoreBookkeepingPath` deliberately repeats the narrower, safer pattern rather than reusing a broader existing helper.
- `D-3` **`advanceFlowThroughFreezeChain` now always builds and renders a `FlowContextPackage` for the writer, regardless of whether the freeze node's path to it passes through an intermediate `context.produce` hop.** The original implementation only built (and, critically, only ever rendered) a package when the path being walked contained a mid-hop inline node — for review-loop, whose freeze node edges directly to `coder`, this meant no context, no source excerpts, and no feature history ever reached the coder's actual prompt, only the frozen contract's bare intent sentence. This was found only while fixing test fallout from the migration itself (see Trigger) — every P-8 e2e test that checks context correctness uses a fixture WITH a context node, which structurally could not have caught this gap for review-loop's own shape.
- `D-4` **Review-loop keeps its pre-migration topology of having no `context.produce` node at all, rather than adding one to match rag-harness/context-coding-review-synthesis's shape.** The CP-55 spec's own illustrative target pattern names a fixed `context: context.produce` step, but D-3's fix makes the no-context-hop case fully safe regardless of whether such a node exists — adding one to review-loop purely for shape uniformity was judged out of scope for this phase. Recorded explicitly as a known, disclosed asymmetry (CA-431), not a silent inconsistency.
- `D-5` **The shared fake-adapter fix in `fake_provider_adapter.go` is a real demo-mode correctness fix, not test-only scaffolding.** This exact adapter backs `DefaultProviderRegistry`'s Codex fallback in production whenever the real Codex app-server path is disabled ("demo/tests stay green without a codex binary" — `ProviderRegistryFor`'s own doc comment). Without it recognizing the contract-planner's turn, every migrated flow would fail its freeze step on literally the first turn in demo/no-credentials mode, not only in the test suite.

### Constraints

- All 18 of P-8's own coding-plan test signatures pass.
- Zero new regressions against the established 15 pre-existing/environment `internal/runner` failures — confirmed via a full regression run to completion after every fix pass (three full runs total this phase, each ~6-7 minutes).
- Every test-suite failure caused by the topology change was individually investigated for its actual root cause before being fixed — never assumed to be "just a stale expectation" without confirming the specific failure reason first. One (`TestE2EReviewLoopBlockedByProseOnlyRoundStopsAdvancing`) was found to be passing for the wrong reason and is now fixed to genuinely exercise its intended scenario.
- Provider-agnostic (Case 1) — proven by cross-provider byte-equivalence comparison, not merely absence-of-branch inspection.
- The Settings UI `acceptance_nodes` editor prerequisite noted in the coding plan does not block this slice: the three flows this task migrates are authored in source YAML and mirror-synced, not authored through Settings.

### Open Questions

- **Carried forward, not resolved by this task:** the Settings UI selector/editor for `acceptance_nodes`/`agent.code` node authoring (`FLOW_BEHAVIOR_OPTIONS` in `adminModels.ts`) still does not exist. Whichever future slice first lets a user author their own `agent.code`-bearing custom flow from Settings must add it before or alongside that work.
- **Review-loop's asymmetric topology (D-4)**: candidate cleanup for a later phase to add an explicit `context` node to review-loop for uniformity with the other two flows — not required for correctness given D-3's fix, so left as-is for this phase.

## 1. Goal

Prove the CP-55 preflight-contract mechanism (P-1 through P-7) actually works for the three Flows real users hit — not just in isolated unit fixtures — by migrating them to the target pattern and driving restart, gate, and cross-provider scenarios against the real YAMLs end to end.

## 2. Parent Links

- coding plan: `CP-55` P-8

## 3. Trigger

P-1 through P-7 built every mechanism (contract planner, freeze, gate enforcement, pending-canonical staging/finalization, deterministic history ranking) but left all three built-in Flows on the old topology (writer as entry node, no frozen contract ever created for a real user's actual "bug"/RAG/coding-review-synthesis Flow). Implementing this migration surfaced that the mechanism had never actually been exercised against a real multi-node Flow with the writer NOT as the entry node, which is exactly what uncovered all three production bugs fixed in this task.

## 4. Exact Change

- `internal/runner/gate_hook.go` (**modified**): new `agent.code`/frozen-record Canonical Head path; extended bookkeeping-path exemption.
- `internal/changecontract/pending_head.go`, `internal/changecontract/frozen_scope.go` (**modified**): bookkeeping-path constants/exemption.
- `internal/runner/flow_validate_audit_dispatch.go` (**modified**): `advanceFlowThroughFreezeChain` always builds+renders a `FlowContextPackage` for the writer.
- `internal/runner/retrieval_locus.go`, `internal/runner/context_sources_builtin.go`, `internal/runner/flow_context_package.go` (**modified**): `explicitPaths`/`ResolvedFeatureKey` wiring for the first coder context build.
- `internal/agentpack/flow-pack/agents/contract-planner.md` (**new**), `internal/agentpack/flow-pack/manifest.yaml` (**modified**): the planner agent.
- `internal/agentpack/flow-pack/flows/{review-loop,rag-harness,context-coding-review-synthesis}.yaml` (**modified**): the actual migration.
- `internal/runner/fake_provider_adapter.go` (**modified**): shared demo/test adapter recognizes the planner's turn.
- 5 new test files (18 signatures) + 8 existing test files fixed for topology-change fallout (22 tests total, see CA-431 for the itemized list).

## 5. Touched Areas

- files: 6 modified production files, 6 new production/config files (1 agent prompt, 5 test files below counted separately), 8 existing test files fixed for fallout
- modules: `runner`, `agentpack`, `changecontract`
- routes / tables: none (reuses existing `FrozenStore`/`PendingCanonicalStore`/ledger storage)

## 6. Acceptance Check (DoD)

- [x] `TestReviewLoopHasPreflightBeforeEveryWriter` / `TestRAGHarnessHasPreflightBeforeEveryWriter` / `TestContextCodingReviewSynthesisHasPreflightBeforeEveryWriter` / `TestBuiltInCodingFlowsPassSafetyTopologyValidation` — every migrated flow's writer is forward-dominated by a freeze node on every path from a real entry.
- [x] `TestFrozenContractExistsBeforeFirstContextPackage` / `TestFirstCoderContextUsesCurrentFlowDeclaredPaths` / `TestFirstCoderContextRanksFeatureHistoryByCurrentLocus` — the first coder context build for a freshly frozen contract has real source excerpts and locus-ranked history, not an empty/recency-only fallback.
- [x] `TestValidateFailLeavesCanonicalHeadUnchanged` / `TestReviewContinueLeavesCanonicalHeadUnchanged` / `TestTerminalDoneUpdatesCanonicalHeadExactlyOnce` — Canonical Head mutates exactly once, only on genuine terminal acceptance.
- [x] `TestRestartBetweenFreezeAndCoderPreservesContract` / `TestRestartBetweenCoderAndValidationPreservesPendingCanonical` / `TestRestartDuringTerminalFinalizationConverges` — a simulated restart at each of the three critical junctures converges safely on retry.
- [x] `TestClaudePreflightContractUsesSharedParserAndGate` / `TestCodexPreflightContractUsesSharedParserAndGate` / `TestGrokPreflightContractUsesSharedParserAndGate` / `TestProviderAdaptersReceiveEquivalentFrozenContractPayload` — byte-equivalent frozen contracts across all three providers.
- [x] `TestNormalChatRemainsUsableWithoutFlowGuarantees` — a plain Normal-chat coding turn is completely unaffected by every CP-55 mechanism the migrated flows now use.
- [x] **A frozen `agent.code` writer's gate pass actually stages a Canonical Head update** (found not to be true at all before this task's own fix) — `TestFrozenAgentCodeWriterStagesPendingCanonicalUpdate`, `TestFrozenAgentCodeWriterFinalizesOnTerminalDone`.
- [x] **A frozen writer's second gate pass does not self-block on its own prior pending-canonical staging write** (found to be a real, production-breaking gap) — `TestFrozenAgentCodeWriterSecondGatePassDoesNotSelfTriggerDrift`.
- [x] **The writer's actual delivered prompt contains the rendered `FlowContextPackage`, including for a flow whose freeze node edges directly to the writer with no intermediate context node** (found to be false for review-loop's own shape before this task's own fix) — `TestFlowCodingPromptSpawnWrappedDoesNotDuplicateHistory` (pre-existing test, not one of this task's own 18, but the one whose failure led to discovering the gap).
- [x] All 22 test-suite fallout fixes individually root-caused, not assumed — see CA-431's itemized list.
- [x] `go build ./...` clean; `gofmt -l` flags on touched files confirmed to be pre-existing CRLF churn via `git diff --stat`, not genuine formatting issues.
- [x] **Full `internal/runner` regression suite run to completion three times** across this phase's fix pass: exactly the established 15 pre-existing/environment failures on the final run, zero new deterministic regressions, one confirmed-unrelated transient hang (`TestEnsureJiraMcpProviderConfigReusesPersistedBearerTokenOnRerun`, passes in isolation, file untouched by any CP-55 work).
- [x] Cross-provider Case 1 agnostic, proven by byte-equivalence comparison.
- [x] **Claude-agent adversarial review performed and acted on**: 1 Critical + 3 Important + 2 Minor findings; all 4 Critical/Important findings fixed and re-verified via mutation testing (including a real, concretely-demonstrated Canonical Head forgery bypass, closed with an HMAC-signature scheme reusing the existing `markerSecret` trust boundary); both Minor findings explicitly accepted/documented with reasoning in CA-431 — none silently dropped.

## 7. Out of Scope

- The Settings UI `acceptance_nodes`/`agent.code` authoring editor (coding-plan's own explicit carry-over note) — not required for this task since the three flows migrated here are source-YAML-authored, not Settings-authored.
- Adding an explicit `context.produce` node to review-loop for topological uniformity with the other two migrated flows (D-4) — correctness does not depend on it given the `advanceFlowThroughFreezeChain` fix.
- CP-55 P-9 (documentation/rollout evidence/operator review).

## 8. Cross-Provider Note

Provider-agnostic (Case 1), proven by `TestProviderAdaptersReceiveEquivalentFrozenContractPayload`'s pairwise byte-equivalence comparison across Claude/Codex/Grok, not merely a `grep` absence-of-branch check (though that check also returns zero matches over every production file this task touches).
