# Task-266: Enforce Frozen Scope At Coder Gate And Amendments (CP-55 P-4)

## Metadata

- Document ID: `Task-266`
- Title: `Enforce Frozen Scope At Coder Gate And Amendments`
- Phase: `task`
- Status: `done` (2026-07-31 — implemented, then reviewed by a dedicated Claude reviewer agent: 2 Critical, 5 Important, 4 Minor findings, including a real security hole (the original doc/audit-path filter exempted the entire `.flowpilot/**` tree, letting a writer silently rewrite its own gate rules or forge its own frozen contract with zero drift detected) and a fail-open bug (a git-observation failure degraded to trusting AI-self-reported paths instead of blocking). All Critical + Important findings fixed and re-verified via mutation testing (5 separate fixes, each proven by temporarily reverting it and confirming the corresponding test fails, then restoring it). Full `internal/runner` regression re-run after fixes: same 15 pre-existing failures, 0 new. See [CA-427](../../../change-audit/CA-427-enforce-frozen-scope-at-coder-gate-and-amendments.md) for the complete findings/fix accounting.)
- Owner: `FlowPilot Architecture`
- Reviewers: `Claude (agent review, complete — see CA-427)`
- Created: `2026-07-31`
- Last Updated: `2026-07-31`
- Feature Keys: `agent-flow-engine`, `change-contract`
- Parent Documents: [CP-55: Flow-First Preflight Contract, Context Retrieval, and Canonical Acceptance](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md) (P-4), [Task-265](./Task-265-Contract-Planner-Freeze-Node-And-Inline-Chain-Advancement.md) (P-3 — `runContractFreezeNode`/`FrozenContractRecord` this task consumes)
- Child Documents: `none`
- Related Documents: [CA-425](../../../change-audit/CA-425-frozen-preflight-contract-model-and-storage.md), [CA-426](../../../change-audit/CA-426-contract-planner-freeze-node-and-inline-chain-advancement.md)
- Replaces: `None`
- Tags: `agent-flow-engine, change-contract, gate, scope-drift, amendment, additive`

## AI Quick View

### Summary

Wires the frozen contract CP-55 P-3 produces into `agent.code`'s own completion gate (`runChildArtifactOutputGateAtEpoch`): loads the `FrozenContractRecord` bound to the writer's step, computes what it actually wrote (diffed against the frozen record's own `BaseSHA`, filtered of `.flowpilot/`/doc/audit paths), and hard-blocks on any path outside `DeclaredPaths`. This is a new, disjoint branch keyed strictly on `Behavior == "agent.code"` — the legacy `isDelegate`/`prepareChangeContract`/`commitChangeContract` path (used by every real flow today, all of which use `agent.delegate`) is untouched, so post-turn declaration/inference can no longer satisfy Flow preflight for a code writer, while Normal chat and every existing `agent.delegate` flow keep their exact current behavior. Also adds `changecontract.AmendFrozenContract` — a version-bump-and-supersede function for widening a frozen contract's scope before a retry, used only when new paths are genuinely needed (a same-scope retry reuses the existing version unchanged).

### Current Ask

Implement P-4 exactly: split Flow contract preparation from legacy/Normal, `agent.code` loads its bound frozen version, remove post-turn declaration/inference as a Flow-preflight satisfier, scope drift blocks (not warns) for Flow, and an amendment transition for genuine scope-widening retries — while preserving Normal chat's current behavior byte-for-byte.

### Key Decisions

- `D-1` **A new, disjoint branch in the existing gate function, not a rewrite of it.** `runChildArtifactOutputGateAtEpoch` gains one new `if isCodeWriterNode` block (`canonical == "agent.code"` specifically) inserted before the existing `isCodingChild`/`prepareChangeContract` block's early-return, evaluated unconditionally — the same hardcoded-check pattern this function already uses for its git-commit-detection check, rather than routing through the configurable `flowgate` rules engine (whose r-scope rule deliberately downgrades a configured block to warn unless `HighSeverity` — Normal chat's leniency the spec explicitly does not want inherited here).
- `D-2` **`agent.code` never reaches `prepareChangeContract`/`commitChangeContract` at all.** `isDelegate` (and therefore `isCodingChild`) is defined by `canonical == "agent.delegate"` only — unchanged. Since an `agent.code` node is never `isDelegate`, it structurally cannot reach the legacy declare/infer path, which is what makes "post-turn declaration/inference cannot satisfy Flow preflight" true by construction rather than by an extra check.
- `D-3` **Comparison uses the frozen record's own `BaseSHA`, not the turn's `turnStartGitHead`.** These are normally the same commit, but the frozen record does not carry an assumption that they match — `flowgate.ObserveGitDiffSince(cwd, rec.BaseSHA)` is called explicitly rather than reusing the turn-scoped `diff` already computed earlier in the function.
- `D-4` **Only the frozen store's own two exact bookkeeping files are exempt from the written-paths comparison — nothing else.** Caught by the test suite itself before review, not assumed: freezing a contract writes `.flowpilot/contracts/frozen_contracts.ndjson` into the very same workspace being diffed, which a naive comparison flags as coder drift on *every single* `agent.code` turn. The **first** fix filtered this via `flowgate.IsDocOrAuditFile`, which exempts the entire `.flowpilot/**` tree and every `*.md` file — Claude-agent review (CA-427 Finding 2) correctly identified this as a real security hole (a writer could silently rewrite `.flowpilot/settings/flow-rules.json`, disabling the very gate judging it, or forge `frozen_contracts.ndjson` directly, with zero drift ever detected). Fixed properly: `changecontract.IsFrozenStoreBookkeepingPath` recognizes only the exact two files `FrozenStore` itself writes; every other `.flowpilot/**` path and every doc file remains fully subject to enforcement. Verified via mutation testing at every stage (each fix temporarily reverted, the corresponding test — including two new ones added specifically to close this hole — confirmed to fail, then restored to pass).
- `D-5` **`AmendFrozenContract` (new, in `changecontract`) is a pure library function, not wired into an automatic retry loop in this task** — but it is now unit-tested directly in its own package (`frozen_scope_test.go`, added after CA-427 Finding 9 flagged the absence of package-level tests), not only indirectly through the runner gate. It supersedes an existing record with a higher version whose `DeclaredPaths` is the normalized union of the old ones and any genuinely new paths; if the union contributes nothing new, it returns the existing record unchanged — no version bump, no status event. A non-blank `additionalPaths` entry that is not a concrete code target (e.g. `"Makefile"`, extension-less) is now rejected with an explicit error rather than silently dropped (CA-427 Finding 5 — the original behavior was indistinguishable from "nothing needed to change," which would have looped a scope-drift retry forever with no actionable signal). The new record is now saved *before* the old one is marked superseded (CA-427 Finding 4 — the reverse order left a crash/error window where the old version was superseded with no active successor, permanently blocking the step). No UI/automatic-retry surface calls this yet. **Wiring the amendment transition into an actual retry/escalation UX is explicitly not done in this task** (CA-427 Finding 7) — see Open Questions.
- `D-6` **Enforcement is derived from the frozen record's own existence, not solely from live in-memory topology resolution.** The original implementation gated the entire check on `flowNodeForRun` successfully resolving the node AND its behavior being `agent.code` — CA-427 Finding 3 correctly identified that a runner restart, a topology reload, or any other reason that lookup fails would silently skip enforcement entirely for a step a frozen contract really was bound to (the exact "silently let an unbound write through" failure this check exists to prevent). Fixed: the coder-step id (`rs.label`, falling back to `rs.stepID`) is looked up directly against `FrozenStore.GetFrozenForStep` regardless of whether the live node resolves; enforcement fires if *either* the node resolves as `agent.code` *or* a frozen record actually exists for that step. A step that was never frozen (every existing `agent.delegate` flow) still correctly skips enforcement either way.
- `D-7` **The diff-observation failure path fails closed, not open.** The original implementation degraded to `fin.ChangedFiles` (AI-tool-call-self-reported, frequently empty) when the diff against the frozen contract's own `BaseSHA` could not be observed — CA-427 Finding 1 correctly identified that an empty `ChangedFiles` list in that case meant zero drift detected, i.e. the turn passed with nothing actually verified, exactly the ground truth this check exists to establish. Fixed: an observation failure now blocks + escalates, matching this same function's own pre-existing turn-scoped-diff-failure precedent a few dozen lines earlier in the file.

### Constraints

- Additive-only diff to `gate_hook.go`: one new branch inserted, no existing block's logic changed. New files `changecontract/frozen_scope.go` + `frozen_scope_test.go` (13 tests, added post-review); test file `flow_frozen_scope_gate_test.go` (19 tests after the review-fix pass: 14 matching the CP-55 P-4 test signatures + 5 added to close review findings).
- Full `internal/runner` regression suite run twice (original pass and again after the review-fix pass), mandatory per the same CRITICAL-risk classification established for P-3, since `runChildArtifactOutputGateAtEpoch` is the completion gate for every Flow child in the system — GitNexus again misreported it as LOW/0-impacted.
- Provider-agnostic (Case 1).

### Open Questions

- **Carried forward from Claude-agent review (CA-427 Finding 7):** `AmendFrozenContract` has no production caller — wiring it into an actual scope-drift-triggered retry/escalation UX (vs. a plain hard block) is left for whichever later slice adds Flow-driven retry orchestration for `agent.code`. This task proves the amendment mechanism itself behaves correctly (higher version, correct supersede-with-crash-safe ordering, explicit rejection of a non-concrete widen path, no-op on no genuinely new paths) but does not claim it is reachable from a live retry flow yet.
- **Carried forward from Claude-agent review (CA-427 Finding 11):** the drift window is "everything since the contract was frozen," not "everything this specific coder turn wrote" — anything written between freeze and this turn's end by anyone else (a prior retry attempt, a human editing the workspace) is attributed to this coder. This may be the intended semantic for a Flow's acceptance-time guarantee, but it is a materially different rule from "what the coder touched," and whichever slice wires amendment/retry orchestration should make this explicit rather than inherit it silently.

## 1. Goal

Make "a Flow writer cannot write outside its frozen contract, and cannot talk its way around that after the fact" an enforced, tested property of the coder-completion gate.

## 2. Parent Links

- coding plan: `CP-55` P-4

## 3. Trigger

CP-55 P-3 (Task-265) froze a contract and spawned the writer, but nothing at the writer's own completion ever reads that frozen record back — an `agent.code` node's turn was, until this task, entirely ungated (it is not `isDelegate`, so it never even entered the legacy tier-1 path).

## 4. Exact Change

- `internal/changecontract/frozen_scope.go` (**new**): `FrozenContractScopeDrift(rec, writtenPaths) []string`, `AmendFrozenContract(store, workspace, existing, additionalPaths, now) (FrozenContractRecord, error)`.
- `internal/runner/gate_hook.go` (**modified, additive**): one new `if isCodeWriterNode` block inside `runChildArtifactOutputGateAtEpoch`, evaluated before the existing `isCodingChild` block's early-return; no existing line changed.
- `internal/runner/flow_frozen_scope_gate_test.go` (**new**): 19 tests after the review-fix pass — 14 matching each CP-55 P-4 test signature, plus 5 added to directly regression-test the Critical/Important review findings (Findings 1, 2×2, 3).
- `internal/changecontract/frozen_scope_test.go` (**new, post-review**): 13 unit tests for `FrozenContractScopeDrift`, `IsFrozenStoreBookkeepingPath`, and `AmendFrozenContract` directly (Finding 9).

## 5. Touched Areas

- files: 2 new production files (`frozen_scope.go`, rewritten once post-review), 1 modified production file (additive-only), 2 new test files
- modules: `changecontract`, `runner` (gate)
- routes / tables: none (reuses Task-264's `frozen_contracts.ndjson`/`frozen_contract_events.ndjson`)

## 6. Acceptance Check (DoD)

- [x] No frozen contract → block — `TestFlowCoderRequiresFrozenContract`.
- [x] Post-turn declaration cannot substitute for a missing frozen contract — `TestFlowCoderRejectsPostTurnDeclarationWithoutFrozenContract`.
- [x] Frozen scope (not the final message's own claim) governs the pass/fail decision — `TestFlowCoderUsesFrozenScopeInsteadOfFinalMessage`.
- [x] Written-paths comparison uses the frozen record's own `BaseSHA` — `TestFlowCoderComputesWrittenPathsAgainstFrozenBaseline`.
- [x] Scope drift blocks — `TestFlowScopeDriftBlocksAcceptance`; message names the exact unexpected path(s) — `TestFlowScopeDriftReportsExactUnexpectedPaths`; cannot be satisfied by a post-turn declaration of the drifted path — `TestFlowScopeDriftCannotBeSatisfiedByInference`.
- [x] Amendment creates a higher version — `TestFlowContractAmendmentCreatesHigherVersionBeforeRetry` (now asserted against a freshly re-opened store, corrected after CA-427 Finding 8); supersedes the prior one, crash-safely — `TestFlowContractAmendmentSupersedesPriorVersion`; a same-scope retry reuses the existing version, no pointless amendment — `TestRetryWithoutNewPathsReusesFrozenVersion`; a non-concrete widen path is rejected explicitly, not silently dropped — `TestFlowContractAmendmentRejectsNonConcreteAdditionalPath` (new, Finding 5).
- [x] Normal chat / `agent.delegate` legacy behavior unaffected — `TestNormalChatKeepsLegacyDeclaredContractBehavior`, `...InferredContractBehavior`, `TestNormalChatCodeRequestIsNotBlockedForMissingFlow`.
- [x] Enforcement is independent of the `change.contract` context source's own render/disable state — `TestChangeContractSourceDisabledDoesNotDisableFlowGate`.
- [x] **Only the frozen store's own two exact bookkeeping files are excluded from the drift comparison — nothing else** (corrected from the original, too-broad `.flowpilot/**`/doc-wide exclusion after CA-427 Finding 2, a real security hole). Verified via mutation testing (each fix temporarily reverted, the corresponding test confirmed to fail, then restored to pass) — `TestFlowScopeDriftDetectsRewrittenGateRulesFile`, `TestFlowScopeDriftDetectsForgedFrozenContractFile` (both new).
- [x] **A diff-observation failure against the frozen baseline fails closed** (corrected from a fail-open fallback to AI-self-reported paths after CA-427 Finding 1), verified via mutation testing — `TestFlowCoderBlocksOnUnobservableFrozenBaseline` (new).
- [x] **Enforcement fires even when live topology fails to resolve the writer node**, keyed off the frozen record's own existence for the step (corrected after CA-427 Finding 3), verified via mutation testing — `TestFlowCoderEnforcedEvenWhenLiveTopologyUnresolved` (new).
- [x] `gofmt -l`/`go build`/`go vet` clean on every touched/new file (package-wide `gofmt -l` on `gate_hook.go` flags only the same pre-existing CRLF churn CA-424/425/426 already documented; `git diff --stat` confirms a pure, additive diff at every stage).
- [x] **Full `internal/runner` regression suite run twice** (original pass and again after the review-fix pass): 15 failures both times, identical set to CP-55 P-3's own run (same files, none touched by this task) — zero new failures, zero flakes either run.
- [x] Cross-provider Case 1 agnostic.
- [x] GitNexus CLI impact query run on `runChildArtifactOutputGateAtEpoch` before editing; reported LOW/0-impacted, contradicted by it being the completion gate for every Flow child in the system — same disclosed-and-proceeded pattern as P-3.
- [x] **Claude-agent adversarial review performed and acted on**: 2 Critical + 5 Important + 4 Minor findings; both Critical findings and 4 of the 5 Important findings fixed and re-verified via mutation testing; the remaining Important findings (7, 11) and all Minor findings are explicitly accepted/deferred with reasoning in CA-427, not silently dropped.

## 7. Out of Scope

- Wiring `AmendFrozenContract` into an actual automatic retry/escalation UX (D-5, CA-427 Finding 7).
- Catalog-backed `feature_key` validation at the gate (inherits P-3's `D-7` deferral).
- Redefining the drift window to be per-turn rather than since-freeze (CA-427 Finding 11 — carried as an explicit open question, not silently resolved).
- CP-55 P-5 (Canonical terminal-acceptance timing) through P-9.

## 8. Cross-Provider Note

Provider-agnostic (Case 1): neither the new gate branch nor `frozen_scope.go` reads or branches on a `providerKey`. Confirmed via `grep -inE 'providerKey|claude|codex|grok'` → zero matches in `frozen_scope.go`; the new gate branch reuses the same provider-agnostic helpers (`changedPathsFromDiff`, `flowgate.ObserveGitDiffSince`) the pre-existing gate code already does.
