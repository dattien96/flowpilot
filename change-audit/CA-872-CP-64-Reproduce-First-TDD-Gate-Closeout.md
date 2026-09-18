# CA-872 — CP-64 Reproduce-First TDD Gate closeout (P-1 → P-4)

# ---8<--- flowpilot:change-ledger
feature_key: reproduce-first-gate
source_doc_id: CP-64
change_type: feature
summary: close CP-64 P-1 to P-4 — r-reproduce rule + oracle classifier, reproducer prompt/agent/behavior, bug-flow reproduce node with coder test-file lock, lifecycle E2E red-green proof
# --->8---

## Why

BugFix TDD used empty test signatures filled in later by the Coder itself
(confirmation bias: the test never ran RED, the fix had no physical proof).
CP-64 adds the `r-reproduce` gate so a bug must be demonstrated by an
executable assertion-failure test BEFORE production code may be touched,
then locks that test read-only so the Coder cannot rewrite it to fit the fix.

## Change

- **P-1 rule engine** (`internal/flowgate/reproduce_rule.go` new,
  `oracle.go` + `rules.go` + `gate_hook.go` additive):
  `ReproduceRuleID = "r-reproduce"` (Trigger `reproduce_not_demonstrated`,
  Action `reprompt`), deliberately NOT in `DefaultRules()` — appended only
  when the active node carries behavior `agent.reproduce` (or
  `change_type: behavior-change`) AND `FLOWPILOT_ENABLE_REPRODUCE_GATE` is on.
  `classifySuiteOutput(testCmd, output)` separates compile error (never a
  reproduction) from assertion failure; `TurnResult` gains caller-computed
  `ReproduceExpected` + `ReproduceCompileFailed`; the reproduce turn suppresses
  `r-tests`/`r-reg` exactly once (proposal-turn exemption pattern).
- **P-2 pack** (`flow-pack/prompts/reproduce-failing-test.md` +
  `flow-pack/agents/reproducer.md` new, `manifest.yaml`,
  `behaviors/registry.yaml` reference + `behavior_registry_builtin.go`
  runtime): behavior `agent.reproduce` (scope `delegate`, handler
  `behaviorAgentDelegate`, write-scope test file only), file_artifact OUTPUT
  binding records the reproduction test path. `test-signatures.md` and
  `agents/tester.md` untouched — feature flows keep empty signatures.
- **P-3 flows + lock** (`flows/bug-harness.yaml`,
  `flows/bug-plan-harness.yaml`, `changecontract/frozen_scope.go`,
  `flow_validate_audit_dispatch.go` bridge, `reproduce_gate.go` new,
  `prompts/implement-complete-tests.md` additive 1 paragraph):
  node `test_signatures` replaced by `reproduce_test`
  (`agent.reproduce` + `reproducer.md` + `reproduce-failing-test.md`);
  after a passing reproduce turn the test path is written into the coder
  step's frozen record as `ReadOnlyPaths` (`recordReproduceTestLock`) and
  enforced silent-deny at `turnBridge.RequestApproval`
  (`reproduceLockCommandTargetsLockedPath`, `IsReadOnlyLockedPath`).
  Flag off → `resolveReproducePrompt/Agent` degrade to the legacy empty-frame
  mechanism at runtime (no gate, no lock). Only the 2 bug flows changed;
  `rag/task/vibe` flows byte-identical.
- **P-4 E2E** (`internal/runner/bug_fix_reproduce_e2e_test.go` new, test-only):
  `TestBugFixLifecycleEndToEndWithReproduceGate` (RED oracle proof →
  reproduce pass + lock → coder write-to-test denied → fix → GREEN with the
  reproduce test flipped Failed→Passed, zero regressions),
  `TestBugFixFailsClosedWhenBugNotReproduced` (green-on-arrival /
  compile-error / EnvError-bogus-binary — all reprompt at `reproduce_test`,
  implement never dispatched, production diff empty),
  `TestTaskHarnessSignatureTurnPassesWithReproduceGateOn` (T-3/AC-3),
  `TestBugHarnessLegacyPathWithReproduceGateOff` (T-4/AC-6).
  Fixture: seed commit (buggy calc) + committed oracle baseline
  (`.flowpilot/guard/test_baseline.json` tracked in the seed, untracked-model
  preserved — production `BaselineWorktree` snapshot handles real dirt, so no
  baseline exemption was added); `commitBugFixTree` excludes `.flowpilot/`
  (runner bookkeeping is never committed mid-flow).

## Tests

- P-1: `TestRuleReproduceFailsWhenAllTestsPass`,
  `TestRuleReproduceFailsOnCompileError`,
  `TestRuleReproducePassesOnAssertionFailure`,
  `TestRuleReproduceSkipsOnNonBugFlow`, `TestClassifySuiteOutputPerRunner`,
  `TestReproduceRuleOptInAndTestRuleSuppression` — green.
- P-2: `TestReproducerAgentPromptRender`, `TestReproducerArtifactBinding`,
  `TestReproduceBehaviorRuntimeBinding` — green.
- P-3: `TestBugHarnessTopologyContainsReproduceGate`,
  `TestBugPlanHarnessTopologyContainsReproduceGate`,
  `TestCoderNodeHasTestFileAsReadOnly` — green.
- P-4: 4 E2E above — green (mock provider turns + REAL go toolchain/oracle).
- Full packages: `agentpack` green, `changecontract` green, `flowgate` green
  except 2 pre-existing Windows `.sh`-fixture failures
  (`Test540927Oracle*`, `%1 is not a valid Win32 application` — files untouched
  by CP-64). `runner`: all CP-64 tests green; pre-existing reds
  (`TestRun144900_*` skill-drift baseline, `TestRootFlowEngineDefers...`)
  live in files outside the CP-64 diff — R1 STOP, not touched.
- Debug instrumentation reverted (`debug-e2e` grep = 0).

## Providers

Provider-agnostic. Evidence: grep `providerKey|ProviderKey` over
`reproduce_rule.go`, `oracle.go`, `reproduce_gate.go`, `gate_hook.go`,
`frozen_scope.go` = 0 hits. E2E fixtures use one representative
`ProviderKeyCodex` for `createRun` only (allowed per Task-367 constraints);
no per-provider branching exists in the gate, classifier, lock, or prompts.

## Prior CA claims kept intact

- No prior CA exists for `reproduce-first-gate` (new key, registered in
  FEATURE-KEYS.md before this note); nothing superseded.
- CP-55 frozen-contract semantics preserved (`ReadOnlyPaths` is a zero-value
  compatible additive field; DeclaredPaths of the coder holds production
  paths only). CP-62 gate precedence untouched (`r-reproduce` appends
  selectively, `ResolvePrecedence` routing unchanged).
- Task-364/365/366/367 Completion Notes point here; CP-64 DOD §10 ticked.
