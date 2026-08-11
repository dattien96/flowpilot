# Task-265: Contract Planner, Freeze Node, And Inline-Chain Advancement (CP-55 P-3)

## Metadata

- Document ID: `Task-265`
- Title: `Contract Planner, Freeze Node, And Inline-Chain Advancement`
- Phase: `task`
- Status: `done` (2026-07-31 — implemented, then reviewed by a dedicated Claude reviewer agent per operator direction: 4 Critical, 11 Important, 7 Minor findings; all 4 Critical plus the in-scope Important findings fixed and re-verified, including a full `internal/runner` regression run after the fix pass. See [CA-426](../../../change-audit/CA-426-contract-planner-freeze-node-and-inline-chain-advancement.md) for the complete findings/fix accounting.)
- Owner: `FlowPilot Architecture`
- Reviewers: `Claude (agent review, complete — see CA-426)`
- Created: `2026-07-31`
- Last Updated: `2026-07-31`
- Feature Keys: `agent-flow-engine`, `change-contract`
- Parent Documents: [CP-55: Flow-First Preflight Contract, Context Retrieval, and Canonical Acceptance](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md) (P-3), [Task-263](./Task-263-Explicit-Flow-Writer-Semantics-And-Safety-Topology.md) (P-1 — `agent.code`/`contract.freeze` behavior ids), [Task-264](./Task-264-Frozen-Preflight-Contract-Model-And-Storage.md) (P-2 — `FreezeContract`/`FrozenStore`)
- Child Documents: `none`
- Related Documents: [CA-424](../../../change-audit/CA-424-explicit-flow-writer-semantics-and-safety-topology.md), [CA-425](../../../change-audit/CA-425-frozen-preflight-contract-model-and-storage.md)
- Replaces: `None`
- Tags: `agent-flow-engine, change-contract, contract-planner, freeze-node, inline-chain, additive`

## AI Quick View

### Summary

Wires the P-2 `changecontract.FreezeContract`/`FrozenStore` API into the actual Flow dispatch loop: a new `contract.freeze` case in `tryAdvanceFlowThroughInline`'s switch, backed by `runContractFreezeNode`, which enforces the read-only planner produced no mutations, strictly parses/validates its JSON proposal, resolves the writer this contract binds to (walking through zero or more intermediate inline nodes such as `context.produce`, bounded by a hop limit with cycle detection), freezes the contract durably (reload-verified before advancing), and spawns the writer as a child agent run. Adds `apps/local-runner/internal/agentpack/flow-pack/agents/contract-planner.md`, the read-only planner prompt.

No built-in flow YAML declares `contract.freeze`/`agent.code` yet (CA-424 confirmed this for P-1; still true here), so this new code path is exercised only by hand-built test fixtures — the change is structurally a no-op for every existing flow, the same "safe superset" shape P-1's topology validator has.

### Current Ask

Implement P-3 exactly: the planner prompt, `runContractFreezeNode`, the generalized bounded inline-chain advancement (`resolveFreezeWriterTarget` + `advanceFlowThroughFreezeChain`), planner-mutation enforcement, and persist-before-advance. Per operator instruction this session, implementation and review both run inside this session — review is a dedicated Claude agent invocation (not Codex), not yet performed as of this doc's `done` status.

### Key Decisions

- `D-1` **`contract.freeze`'s real logic lives beside `runValidateNode`/`runAuditNode`, not in the `BehaviorRegistry`.** The registry's `BehaviorContractFreeze` entry stays the fail-closed `behaviorContractFreezeNotImplemented` stub from Task-263 — unreached in practice, exactly like `command.validate`/`artifact.audit_draft`/`telegram.notify`/`hub.notify` already work. `runContractFreezeNode` is dispatched directly from a new `case "contract.freeze":` in `tryAdvanceFlowThroughInline` (`flow_validate_audit_dispatch.go`), matching every other inline behavior's actual dispatch path in this codebase.
- `D-2` **The chain-resolution logic is pure and separately testable from the dispatch/I-O logic.** `resolveFreezeWriterTarget(edges, nodes, fromNodeID, hopLimit)` is a plain graph walk with no side effects — it returns the writer node, the ordered list of intermediate inline hops, and whether resolution succeeded. `TestInlineChainStopsAtHopLimit`/`TestInlineChainDetectsCycle` exercise it directly with hand-built fixtures, with no service/provider/git setup needed.
- `D-3` **Bounded to 6 hops (`flowInlineChainHopLimit`).** No spec number was given; this is deliberately generous (no built-in topology needs more than one or two intermediate inline nodes) while still failing closed on a cycle or a pathologically long chain instead of hanging forever.
- `D-4` **Planner-mutation enforcement diffs a flow-start worktree fingerprint against a fresh one at freeze time, not a plain "is anything dirty" check.** The first implementation used `changedFilesSince(workspace, baseSHA)`, matching `runAuditNode`'s own reuse of that helper — but Claude-agent review (CA-426 C-1) correctly identified that `changedFilesSince` reports *every* currently-uncommitted path regardless of `baseSHA`, so it would fire on any pre-existing dirty file at flow start, not just the planner's own turn. Fixed: `interactiveRun.flowStartWorktreeFingerprint` is captured once in `startResolvedFlow` (alongside `flowStartGitHead`), and `runContractFreezeNode` diffs a fresh `baselineWorktreeFingerprint` against it via the new `worktreeMutatedSincePaths` — a path absent from the baseline, or present with a different content hash, is the planner's own mutation; a path dirty in both with the same hash is pre-existing and not the planner's doing.
- `D-5` **`BaselineWorktree` fingerprinting (`baselineWorktreeFingerprint`) is the runner-side capture Task-264's own design decision (D-2) deferred to P-3, and now does double duty as the flow-start baseline `D-4` diffs against.** It hashes the current on-disk content of every `uncommittedChangedPaths` entry (sha256 hex); an unreadable file gets an empty-string entry rather than failing the freeze, and a clean worktree yields `nil` — matching `FrozenContractRecord.BaselineWorktree`'s `omitempty` contract. Its inherited 20-path cap and unbounded-read behavior (both from `uncommittedChangedPaths`) are known, accepted limitations out of this task's scope — see CA-426 I-7.
- `D-6` **Duplicate delivery is de-duplicated at the frozen-contract layer, not the physical-spawn layer — and the check-then-decide sequence is now serialized in-process.** `runContractFreezeNode` checks `FrozenStore.GetFrozenForStep` before computing a new version, holding a new per-`(runID, freezeNodeID)` in-process mutex (`runContractFreezeNodeLockFor`, added after CA-426 I-1 identified a same-process TOCTOU race across two separately-opened `FrozenStore` instances) across the whole check-freeze-save decision; if a version already exists, it is reused verbatim rather than minting a second one. The subsequent writer spawn in `advanceFlowThroughFreezeChain` is **still not** de-duplicated against an already-in-flight child — this matches the existing, already-shipped precedent in `tryAdvanceFlowFromNode`, which likewise always spawns fresh unless a node explicitly declares `Lifecycle: reinvoke` (`flowNodeReusesChild`); de-duplicating physical spawns on redelivery remains a general Flow reinvoke/reconciliation concern (CP-51's domain). Distinguishing genuine redelivery from deliberate re-planning on a retry loop (CA-426 I-5) is explicitly deferred to CP-55 P-4's amendment mechanism — see Open Questions.
- `D-7` **`knownFeatureKeys` is passed as `nil` to `ValidatePreflightDraft`, deliberately.** Task-264/P-2's `ValidatePreflightDraft` already supports catalog-backed allowlisting, but wiring a live `featurecatalog.Catalog` lookup into the freeze dispatch is not required by any P-3 test signature and is left for a later slice — the planner's declared `feature_key` is trusted as-is for now. No API changed to make this possible later.
- `D-8` **`context.produce` is dispatched for real when it appears as an intermediate hop, not skipped as a no-op.** `advanceFlowThroughFreezeChain` calls `BuildFlowContextPackageWithSources` with hints derived from the frozen contract's `Intent`/`DeclaredPaths`, then emits the same `EventFlowContextPackage` / caches on `rs.planContextPackage` that the pre-existing `injectFlowContextIfCoding` slow path already does (`flow_context_handoff.go`) — so the eventual coder turn reuses this package instead of rebuilding one from scratch. This does not yet feed `RetrievalLocus`-based ranking (CP-54/CP-55 P-7's job); it produces the same kind of package the legacy per-turn injection already builds.

### Constraints

- No built-in flow YAML is migrated in this task (CP-55 P-8's job) — `contract.freeze`/`agent.code` nodes exist only in hand-built test fixtures here.
- Additive tests only, in a new file (`flow_contract_freeze_test.go`, 21 tests after the review-fix pass); the production files touched (`flow_validate_audit_dispatch.go`, plus `interactive_service.go`/`flow_executor.go` after the review-fix pass for the flow-start worktree fingerprint) gained only new imports, one new `switch` case, updated doc comments, new top-level functions, and one new struct field + its one-line capture — no existing function body or struct field was modified.
- Full `internal/runner` regression suite run (not a subset) given this touches the shared inline-dispatch switch, per explicit operator confirmation after being warned GitNexus's CLI impact score for `tryAdvanceFlowFromNode`/`tryAdvanceFlowThroughInline` (LOW/0) does not match the manually-confirmed reality (both are called from the run-completion path and covered by ~20+ existing test call sites — treated as CRITICAL by manual judgment, matching `NormalizeBehaviorID`'s precedent in CA-424).
- Provider-agnostic (Case 1) — no `providerKey`/`claude`/`codex`/`grok` branch in any new function.

### Open Questions

- None blocking this task's own exit condition. P-4 needs to decide how `agent.code`'s own gate compares actual changed paths against the frozen record this task produces (currently nothing reads `FrozenContractRecord` back at the writer's own completion — that is explicitly P-4's "enforce frozen scope at the gate" scope).
- **Carried forward from Claude-agent review (CA-426 I-5):** the current reuse-on-duplicate-delivery check (`D-6`) cannot distinguish "the same freeze-node completion redelivered" from "the planner deliberately re-ran on a retry loop with a genuinely different proposal" — both key off the same `(runID, coderStepID)` pair with no round/attempt dimension. P-4's amendment mechanism is the natural place to resolve this: a retry that wants a wider/different scope should go through an explicit amendment (new version, `Supersedes` set), not silently have its new proposal discarded by this task's blunt reuse check.
- **Carried forward from Claude-agent review (CA-426 I-9):** `escalate()`'s `applyFlowControl` failure is logged but not itself surfaced as a distinct flow state — this matches every sibling function in `flow_validate_audit_dispatch.go` (`runValidateNode`/`runAuditNode`) and was not treated as a new gap introduced by this task, but it remains true that a double-failure (escalate AND applyFlowControl both failing) leaves the flow in a state with no user-visible signal beyond the log line. Worth a project-wide look, not scoped to P-3 alone.

## 1. Goal

Make "contract frozen durably before the writer ever runs" an observable, testable property of the Flow dispatch loop, for a topology using `contract.freeze` — not just a P-2 library capability nothing calls.

## 2. Parent Links

- coding plan: `CP-55` P-3
- tech design: `SD-21`, `SD-24` (durable dispatch — the freeze/reload/advance sequence must survive a restart, matching D-6's duplicate-delivery reuse)

## 3. Trigger

CP-55 P-2 (Task-264) built `FreezeContract`/`FrozenStore` but nothing in the executor calls them (P-2's own scope note: "P-2 only creates infrastructure; no context source consumes locus yet"). P-3 is the first slice where a frozen contract can actually come into existence during a real Flow run.

## 4. Exact Change

- `internal/agentpack/flow-pack/agents/contract-planner.md` (**new**, then fixed post-review): read-only planner prompt, `tools: [Read, Grep, Glob]`, strict-JSON-only response contract; the response-format example was moved out of a markdown fence after Claude-agent review (CA-426 I-10).
- `internal/runner/flow_validate_audit_dispatch.go` (**modified**): new imports (`crypto/sha256`, `encoding/hex`, `os`, `sort`, `sync`); one new `case "contract.freeze":` in `tryAdvanceFlowThroughInline`; updated doc comment; `flowInlineChainHopLimit` const; `resolveFreezeWriterTarget`, `worktreeMutatedSincePaths`, `runContractFreezeNodeLockFor`/`runContractFreezeNodeLocks`, `baselineWorktreeFingerprint`, `runContractFreezeNode`, `advanceFlowThroughFreezeChain` — the last four rewritten once, post-review, to fix CA-426 C-1/C-2/C-3/C-4/I-1/I-2/I-4/I-6.
- `internal/runner/interactive_service.go` (**modified, post-review only**): `interactiveRun` gains `flowStartWorktreeFingerprint map[string]string`.
- `internal/runner/flow_executor.go` (**modified, post-review only**): `startResolvedFlow` captures `flowStartWorktreeFingerprint` alongside the pre-existing `flowStartGitHead` capture.
- `internal/runner/flow_contract_freeze_test.go` (**new**): 21 tests (16 original + 5 added post-review) covering the freeze success path, chain advancement (with and without an intermediate `context.produce` hop), hop-limit/cycle rejection including the exact boundary, non-`context.produce`/non-`agent.code` chain-shape rejection, all rejection paths (invalid draft, planner mutation vs. pre-existing dirt, unresolvable writer, and the implicit "no writer dispatch after failure" invariant), recovery/duplicate-delivery reuse (corrected to assert against a post-call store instance), and the baseline-fingerprint helper.

## 5. Touched Areas

- files: 1 new prompt file (+1 post-review fix), 3 modified production files (`flow_validate_audit_dispatch.go` substantially, `interactive_service.go`/`flow_executor.go` minimally — one struct field + one capture line, both post-review), 1 new test file
- modules: `runner` (dispatch, run-state), `agentpack/flow-pack/agents` (prompt)
- routes / tables: none (reuses Task-264's `.flowpilot/contracts/frozen_contracts.ndjson` / `frozen_contract_events.ndjson`)

## 6. Acceptance Check (DoD)

- [x] Freeze persists before any coder spawn is guaranteed to have landed — `TestRunContractFreezeNodePersistsBeforeCoderSpawn`.
- [x] Planner mutation is distinguished from pre-existing dirty state, both directions tested — `TestRunContractFreezeNodeRejectsPlannerChangedFiles` (mutation after flow start blocks), `TestRunContractFreezeNodeAllowsPreExistingDirtyWorktree` (dirt before flow start does not block) — corrected after Claude-agent review found the original check fired on any dirty tree (CA-426 C-1).
- [x] Invalid/incomplete planner draft blocks the freeze — `TestRunContractFreezeNodeRejectsInvalidDraft`.
- [x] No resolvable writer target blocks the freeze — `TestRunContractFreezeNodeRejectsUnknownCoderTarget` (assertion corrected to check the store file doesn't exist, not a vacuous unrelated-id lookup — CA-426 M-2).
- [x] Frozen record's `CoderStepID`/`PlannerStepID` bind to the resolved writer/freeze node ids — `TestRunContractFreezeNodeBindsContractToCoderStep`.
- [x] `BaseSHA` matches the flow-start git HEAD — `TestRunContractFreezeNodeRecordsBaselineSHA`.
- [x] A fresh `FrozenStore` instance (not the one the function used) independently sees the persisted record — `TestRunContractFreezeNodeReloadsDurableRecordBeforeAdvance`.
- [x] The chain advances through an intermediate `context.produce` hop to the writer — `TestRunContractFreezeNodeAdvancesToContextProduce`, and via the actual dispatch switch — `TestInlineChainAdvancesFreezeThenContextThenWriter`.
- [x] Hop limit and cycle both fail closed, tested as pure graph logic, including the exact boundary — `TestInlineChainStopsAtHopLimit`, `TestInlineChainAcceptsExactlyAtHopLimitBoundary`, `TestInlineChainRejectsOneMoreThanHopLimitBoundary` (added after CA-426 M-1 found an off-by-one in the original hop-count description), `TestInlineChainDetectsCycle`.
- [x] A non-`context.produce` intermediate hop and a non-`agent.code` terminal are both rejected, not silently admitted — `TestInlineChainRejectsNonContextProduceIntermediateHop`, `TestInlineChainRejectsNonAgentCodeTerminal` (added to close CA-426 C-3/M-5).
- [x] No writer spawn occurs after a freeze failure — `TestInlineChainDoesNotDispatchWriterAfterFreezeFailure`.
- [x] A recovered flow (frozen record pre-exists from "a prior process") reuses it rather than re-freezing from a new (different) planner draft — `TestRecoveredFlowReusesPersistedFrozenContract`, asserted against a store opened after the call under test (corrected after CA-426 C-4 found the original assertion was against a stale pre-call instance and would pass regardless of whether the reuse logic existed — confirmed via mutation testing).
- [x] Duplicate delivery of the same freeze-node completion does not mint a second version — `TestDuplicateFreezeDeliveryReturnsExistingContract`, same stale-store correction as above.
- [x] `baselineWorktreeFingerprint` behavior — `TestBaselineWorktreeFingerprintEmptyWhenClean`, `...CapturesDirtyContent`.
- [x] `gofmt -l` clean on every touched/new Go file (both the original pass's files and the review-fix pass's additional `interactive_service.go`/`flow_executor.go` insertions, confirmed via `git diff` rather than a package-wide `gofmt -l`, which flags unrelated pre-existing CRLF churn in both files per CA-424/CA-425's own documented finding).
- [x] `go build ./...` and `go vet ./...` clean (same one pre-existing, unrelated `internal/structure/gitnexus.go` vet note CA-425 already recorded).
- [x] **Full `internal/runner` regression suite run twice** (original pass and again after the review-fix pass, not a subset, per the operator-confirmed CRITICAL-risk decision above): 16 failures each time, all independently confirmed pre-existing/environment-specific or single-run timing flakes unrelated to this change — see Verification below and CA-426 for the full accounting.
- [x] Cross-provider Case 1 agnostic — zero `providerKey`/`claude`/`codex`/`grok` matches in the new functions.
- [x] GitNexus CLI impact query run on `tryAdvanceFlowFromNode`/`tryAdvanceFlowThroughInline` before editing; both reported LOW/0-impacted, which manual grep (20+ call sites, dedicated test files) contradicts — flagged to the operator explicitly, who confirmed proceeding with the full-regression-required plan.
- [x] **Claude-agent adversarial review performed and acted on** (per operator direction, replacing Codex for this phase onward): 4 Critical + 11 Important + 7 Minor findings; all 4 Critical and the in-scope Important findings fixed and re-verified (including mutation-testing the two corrected duplicate-delivery tests to confirm they now actually detect the logic they claim to cover), the rest explicitly accepted/deferred with reasoning — full accounting in CA-426.

## 7. Out of Scope

- Enforcing the frozen scope at `agent.code`'s own gate, or amendment/supersede transitions on scope drift (P-4).
- Terminal Canonical mutation timing (P-5). History relevance scoring/ranking wiring (P-6/P-7). Built-in flow migration (P-8). Docs/rollout (P-9).
- Catalog-backed `feature_key` allowlisting at freeze time (`D-7`).
- De-duplicating a physical writer re-spawn on duplicate freeze-node delivery (`D-6`).

## 8. Completion Notes

Implemented and self-verified by Claude in this session, per the operator's explicit instruction to implement all remaining CP-55 phases sequentially and have each reviewed by a dedicated Claude agent invocation rather than Codex. The dedicated Claude-agent review pass ran next and found 4 Critical, 11 Important, and 7 Minor findings against the original implementation — all 4 Critical findings were real bugs (not false positives): a planner-mutation check that fired on any pre-existing dirty workspace rather than the planner's own changes, a spawn-failure path that could leak an unbound-writer situation to the legacy hub fallback, silent no-op execution of any non-`context.produce` inline hop, and two duplicate-delivery tests that asserted against a stale store instance and would have passed with the logic they claimed to test entirely removed (verified directly via mutation testing — disabling the logic made both tests fail, restoring it made them pass again). All four, plus five of the eleven Important findings that were actionable within this task's own scope, are fixed in this same document/CA-426; the rest are recorded as explicitly accepted/deferred with reasoning (mostly pointing at CP-55 P-4's amendment mechanism or shared-helper limitations out of this task's scope), not silently dropped. A full `internal/runner` regression run was repeated after the fix pass and showed the same pre-existing environment-failure set as before, plus one different (but equally non-reproducing-in-isolation) timing flake — no regression attributable to the fixes.

## 9. Cross-Provider Note

Provider-agnostic (Case 1): `runContractFreezeNode`/`advanceFlowThroughFreezeChain`/`resolveFreezeWriterTarget`/`baselineWorktreeFingerprint` take no `providerKey` and branch on none. Confirmed via `grep -n providerKey|ProviderKey` on `flow_validate_audit_dispatch.go` returning zero matches inside the new functions (the file's pre-existing `composeHubNotifyPrompt` content, untouched by this task, is unrelated).
