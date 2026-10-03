# BUG-629 — `go test ./internal/runner` has ~21 persistent failures on main (env-dependent + broken assertions), masking real regressions

- **ID:** BUG-629
- **Severity:** Medium (the package suite is red on main; every change
  forces a full manual triage to separate new regressions from noise —
  cost paid in the BUG-624/625 verification, 2026-10-04)
- **Status:** partially resolved — deterministic code-path failures fixed
  (CA-1161); remaining failures are environment-only (missing provider
  binaries, live env leakage, network)
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

1. **Knowledge-bootstrap ↔ test tempdirs** — **FIXED (CA-1159)**:
   `ensureGitNexusIndexAsync` no longer arms under `go test`
   (`runningUnderGoTest`, shared with `opencode_models_cache`), so the
   auto-index never writes `.gitnexus/` into test tempdirs and the
   `TempDir RemoveAll` cleanup race is gone. Verified: the 8 previously
   flaky contract/freeze/advance tests pass. Remaining cleanup debt:
   stale `001` registry entries from past runs were manually removed
   2026-10-04; the bootstrap could still pass the absolute path instead
   of the `001` label to harden future ambiguity.
2. **Suite health** — **FIXED (CA-1161)**: root cause was oracle drift,
   not a code regression. Task-455 (`ad0b66ff`) introduced
   `effectiveGateMode`: a root run without `flowEngineDriven` evaluates
   under `warn` regardless of rule action, so fixtures that re-keyed a
   `newP4ChildRun` onto a plain-chat parent id silently downgraded every
   `block`/`reprompt` assertion. Fixed by restoring the production
   topology each test actually models — enforce-capable child legs for
   `r-ca`/`r-dod` assertions (task331, bug425, bug439_440, bug514),
   `flowEngineDriven` hubs for hub-filter and `r-newtest` reprompt tests
   (cp53 gate-blind, run200816, bug377), and a loop-state assertion
   retargeted to the real gated run (bug514). All 9 cluster tests green;
   `Task455`'s own tests still pin the warn-for-plain-chat semantic.
   Two adjacent leftovers noted: `TestBug425RepromptGateInfersFrom
   CarriedCodePaths` also migrated to a child leg (reprompted legs are
   children), and `TestEnsureGitNexusIndex_AutoAnalyzesOnce` regressed
   under the auto-index guard — fixed with a `FLOWPILOT_GITNEXUS_
   AUTOINDEX=1` opt-in in the stub helper, same escape-hatch convention
   as `FLOWPILOT_OPENCODE_MODELS_CACHE_PATH`/`FLOWPILOT_DEVIN_BIN`;
   `TestBug460AutoIndexSkipsManagedWorktree` opts in too so its
   worktree check is exercised rather than a trivial early return.
3. **Env-dependence**: session/provider tests should `t.Skip` when the
   binary/env is absent instead of hard-failing, so `go test
   ./internal/runner` is meaningful on any dev machine.
4. **Windows-path fixture leaks** (observed 2026-10-04): the suite left a
   literal `C:\working\DnStudio\` directory under `apps/local-runner/` —
   some test feeds a Windows absolute path to a seam that does not
   recognize it as absolute (`filepath.IsAbs` on macOS returns false),
   so it is joined to the runner cwd and materialized. Untracked debris;
   the path-handling seam should reject Windows-style absolute inputs.

## Note

A live Supabase service-role JWT printed into the test log via the
`FlowDefinitionStore` failure — it comes from the developer's own env
config, not the repo, but the test output embeds it verbatim.
