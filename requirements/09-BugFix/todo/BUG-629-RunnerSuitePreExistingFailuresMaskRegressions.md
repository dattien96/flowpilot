# BUG-629 — `go test ./internal/runner` has ~21 persistent failures on main (env-dependent + broken assertions), masking real regressions

- **ID:** BUG-629
- **Severity:** Medium (the package suite is red on main; every change
  forces a full manual triage to separate new regressions from noise —
  cost paid in the BUG-624/625 verification, 2026-10-04)
- **Status:** open
- **Found:** full serial run `go test -count=1 ./internal/runner/` on
  `f7dfd5ab`, 2026-10-04 — 32 failures total, **0 attributable** to the
  BUG-624/625 diffs (same failures reproduced on baseline `4266815f` via
  `git worktree`)

## Failure census (32)

| Cluster | Tests | Cause |
|---|---|---|
| GitNexus registry pollution | ~8 | stale `001` registry entries from
  prior test tempdirs → `RegistryAmbiguousTargetError` → `.gitnexus`
  dirs left in tempdirs → `TempDir RemoveAll` cleanup FAIL.
  **Mitigated 2026-10-04**: removed 3 stale entries + leaked tempdirs
  from `~/.gitnexus/registry.json`; recurrence expected (see below) |
| Missing provider binaries | ~10 | `exec: "opencode"/"codex":
  executable file not found in $PATH` — `TestBug334_*`,
  `TestStartSession*`, `TestCleanupSessions`,
  `TestSweepIdleSessions`, `TestProviderRegistryForUsesLiveWhenFlagOn`,
  `TestCatalogStoreForFallsBackToFake` |
| Env leakage | ~2 | live `SUPABASE_*` env in shell →
  `TestFlowDefinitionStoreForUnconfiguredRunnerYieldsNil` gets a real
  `SupabaseWorkflowFlowStore` instead of nil |
| Pre-existing assertion failures | ~6 | identical on `4266815f`:
  `TestBug377_UnarmedSettleRepromptStillDispatches` (10s waitLoop
  timeout), `TestBug425GateRepromptCarriesFailingTurnCodePaths`,
  `TestBug440RepromptCarryUnionsPartialWriteEvents`,
  `TestBug514_GateRepromptBudgetExhaustedEscalates` (all
  `r-ca` reprompt assertions), `TestGateBlindBlocksTurnEnforceMissingBaseline`,
  `TestRun200816FlowHubGateSkipsTaskDocReprompt` (`r-task`) |
| Firebase E2E | 1 | `TestFirebaseToolsMcpAdapterFetchEndToEnd` —
  needs network/service |

## Defects worth fixing

1. **Knowledge-bootstrap ↔ test tempdirs**: the bootstrap auto-indexes
   each test workspace into the **global** GitNexus registry under the
   basename `001`; stale entries accumulate across runs → ambiguity →
   leftover `.gitnexus` dirs break `t.TempDir()` cleanup. Registry
   writes should be tempdir-scoped/disabled under `go test`, or the
   bootstrap should pass the absolute path instead of the `001` label.
2. **Suite health**: the `r-ca`/`r-task`/gate-blind/`Bug377` cluster
   fails deterministically on main — either the rules moved without the
   tests (oracle drift) or the code regressed without a suite run
   catching it. Needs a bisect + either code fix or test update per the
   additive-tests-only rule.
3. **Env-dependence**: session/provider tests should `t.Skip` when the
   binary/env is absent instead of hard-failing, so `go test
   ./internal/runner` is meaningful on any dev machine.

## Note

A live Supabase service-role JWT printed into the test log via the
`FlowDefinitionStore` failure — it comes from the developer's own env
config, not the repo, but the test output embeds it verbatim.
