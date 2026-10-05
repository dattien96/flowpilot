# CA-1219 — Mirrored step_definitions.model pin strands flow children on unconnected providers

- **Area**: runner / flow-node provider resolution + spawn admission
- **Evidence**: live run-295417 (vibe-adopt, devin hub) — hub spawned
  `contract-planner` with `provider=""` (no override), yet child run-295434
  was created as `codex`/`gpt-5.4` and died ~7s later at adapter
  construction: `no connected local account found for provider "codex"`.
  Same dead child as run-295289/run-295434 both on adopt-sprint.
- **Root cause** (two bypasses of CA-616's "planner inherits hub posture"):

  1. `recordFromWorkflowRow` (supabase_workflow_flow_store.go) carries the
     mirrored `step_definitions.model` onto `node.Model` for delegate/code/
     scaffold nodes — deliberately (Task-320). The CA-616 guard in
     `resolveFlowNodeModel` only skips the step-row *lookup* for the
     planner; the *carried* model is returned verbatim. A stale
     `gpt-5.4` row on `vibe_adopt_sprint_preflight_contract_plan` (confirmed
     via `/client/workflows/{id}/steps` — the adopt-sprint planner/freeze/
     synthesis rows all carry `model=gpt-5.4`, while the hand-maintained
     `vibe_sprint_preflight_contract_plan` carries an operator pin
     `devin/swe-2-high`, which is why CP04 sprints survived) resolved the
     planner to codex — a provider with no connected account on this host.
  2. CA-1218's spawn gate only checked the explicit `in.Provider` channel,
     so a model-derived cross-provider route sailed through and produced a
     dead child run + FAILED flow node + recovery cycle.

- **Fix**:
  - `resolveFlowNodeModel` planner/scout branch: honor the node model pin
    only when `providerKeyFromModel` maps to the parent's own provider —
    a same-provider tier override stays valid (e.g. an admin's
    `devin/swe-2-high` under a devin hub); any cross-provider pin is a
    stranded route and falls back to inherit. The contract-planner is the
    hub's contract voice — its provider must follow the hub.
  - `spawnChildRun`: the CA-1218 account gate now covers the *resolved*
    provider whenever it differs from the parent's — explicit overrides,
    carried node-model pins, and agent-def provider pins all fail before
    the child row exists. Same-provider inheritance is untouched (it can
    never strand — the parent already runs under it). The error carries a
    resolution-source hint: explicit override → "omit the provider
    override to inherit"; resolved pin → "repin it to a connected
    provider's model or connect that provider".

- **Tests** (`ca1219_resolved_provider_gate_test.go`):
  - `TestPlannerCrossProviderCarriedPinInherits` — codex parent + claude
    planner pin → returns "" (inherits hub posture).
  - `TestPlannerSameProviderCarriedPinHonored` — codex parent + gpt pin →
    honored (Task-320 same-provider tier override preserved).
  - `TestSpawnResolvedPinUnconnectedProviderFailsClosed` — model-derived
    claude route with no claude account → spawn error before child row,
    with the resolved-pin hint.
  - `TestSpawnFrozenWriterChildUsesStepDefinitionModel` fixture updated —
    legitimate cross-provider model pins still work when the target
    provider IS connected (planted codex auth).
  - Connected-account fixtures (`isolateProviderHome` +
    `writeTask447Accounts`) added to every pre-existing test that
    legitimately routes cross-provider: tournament candidates
    (`TestBug426_RolloutPassthroughSpawnsCandidates`), reviewer cohorts
    (`TestCoderCompletionAutoSpawnsReviewerCohortWithOwnModel`,
    `TestWorkflowStepsRuntimeReflectsPerNodeModelOverride`), respawn
    routes (`TestBug517_*`), explicit model overrides (`TestBug556_*`).

- **Second defect found via the full-suite flake** (same file family):
  `TestBug426` failed only inside the package suite — candidate-a's
  claude route was rejected with "connected: devin, grok" even though
  the test pinned an isolated home + fixture store. Instrumentation
  showed the fixture file itself had been overwritten with
  `{acct-a(from TestResumeRun...), devin, grok}`: a leaked async
  `ListProviderAccounts` from an earlier test resolves
  `providerAccountsConfigPath()` twice per call — load under the old
  test's env override, save under the *next* test's override —
  clobbering a sibling store with stale state. `HOME` /
  `FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH` are process-global and
  `t.Setenv` rewrites them between tests, so any outlived goroutine
  read-modify-writing the store races every subsequent fixture.
  Fix: `loadProviderAccountStateAt`/`saveProviderAccountStateAt` take
  the path resolved once per public call — a leaked sync can only ever
  write back to the path it actually read. Applies to
  `ListProviderAccounts`, `ConnectProviderAccount`,
  `VerifyProviderAccount`, `ActivateProviderAccount`,
  `DeleteProviderAccount`, `TestProviderAccount`.
  `TestCoderCompletionAutoSpawnsReviewerCohortWithOwnModel` additionally
  drains children to terminal status before TempDir cleanup (the
  verdict-less fake adapter drives the reviewer reprompt/respawn loop
  asynchronously past test end — cleanup raced transcript writes).

- **Adjacent CA-1216 follow-ups swept by the full suite**:
  `vibe-adopt-sprint` added to `builtinHarnessArtifactFlowIDs` (its
  cloned nodes declare the same file_artifact bindings as vibe-sprint —
  BUG-469 mirror seed would silently drop them), and
  `TestPicker_VibeListsOnlyIngest` updated for the fourth picker entry.

- **Live follow-up**: run-295417 wedged with `preflight_contract_plan`
  FAILED and `synthesis` RUNNING (stale-synthesis anomaly still under
  observation). The fix unblocks fresh adopt runs — the planner inherits
  the hub provider — without data surgery on the stale step rows.

- **Suite status**: package suite green for all touched paths; remaining
  full-suite failures are host-environment flakes independent of this
  change (missing `codex`/`opencode`/`agy` binaries, live Supabase
  config resolution under the real HOME, probabilistic gitnexus
  distill→TempDir cleanup races).
