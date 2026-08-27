# CA-648 — FrozenContractScopeDrift gate ignores tool-owned scaffold surfaces

## What

Live F3 flow: the coder's gate pass blocked with

```
flow gate block: flow scope drift: wrote outside the frozen contract's
declared paths: .claude/skills/gitnexus/gitnexus-cli/SKILL.md, ...
.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md, AGENTS.md, CLAUDE.md
```

The writer never touched any of them — the GitNexus skillpack re-installs the
same 8 files mid-flow that CA-645 already exempted at the freeze guard.

## Why

CA-645 fixed only the **freeze planner-mutation guard**
(`isFlowPlannerExcludedPath` in runContractFreezeNode). The **coder's gate**
(`runChildArtifactOutputGateAtEpoch` → `FrozenContractScopeDrift`) filters
written paths with a different, narrower set: the exact bookkeeping files
(FrozenStore / PendingCanonicalStore / RunnerLedger) + CA notes
(`IsChangeAuditPath`) — no scaffold exemption. So the same tool-owned
skillpack churn that no longer parks the freeze still parks the coder gate.

## Fix

- `apps/local-runner/internal/changecontract/frozen_scope.go`:
  - new `ToolOwnedScaffoldPaths()` / `IsToolOwnedScaffoldPath(p)` — the
    single shared definition of tool-owned scaffold surfaces: `.claude/**`,
    `.agents/**`, `.grok/**`, `AGENTS.md`, `CLAUDE.md`, `.gitignore`
    (normalized the same way `FrozenContractScopeDrift` normalizes paths).
    Deliberately does NOT include `.flowpilot/**` or `.gitnexus/**`.
- `apps/local-runner/internal/runner/gate_hook.go`:
  - the code-only filter now also drops `IsToolOwnedScaffoldPath(p)` paths
    before `FrozenContractScopeDrift` runs. CA-427's hole stays closed:
    `.flowpilot/settings/flow-rules.json` and any other `.flowpilot/**` path
    are NOT exempted and still drift (regression-tested).
- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go`:
  - `isFlowPlannerExcludedPath` now delegates the scaffold part to the shared
    `changecontract.IsToolOwnedScaffoldPath` so both guards can never diverge
    again.

Provider-agnostic (Case 1): no `ProviderKey` branch in either guard.

Will not undo: CA-427 (no `.flowpilot/**`-wide exemption — security), CA-640
(runner/gitnexus runtime state), CA-645 (freeze-side scaffold exemption — now
shared), CA-634 (fingerprint subtraction).

## Tests

Additive only — legacy suites untouched.

- `apps/local-runner/internal/runner/ca648_tool_scaffold_gate_exemption_test.go` (new):
  - `TestGateDriftIgnoresSkillpackScaffoldMidFlow` — exact repro: 9 scaffold
    files land during the writer's turn → gate passes.
  - `TestGateDriftStillBlocksRealCodeAlongsideScaffold` — `src/surprise.go`
    out-of-scope still blocks even with scaffold churn.
  - `TestGateDriftStillDetectsRewrittenGateRulesFile` — CA-427 guard:
    `.flowpilot/settings/flow-rules.json` rewrite still drifts.
  - `TestIsToolOwnedScaffoldPathMatrix` — exclusion/kept matrix incl. Windows
    slash and `./`-prefixed forms.

## Verification

- `go test ./internal/runner -run 'TestGateDrift|TestIsToolOwnedScaffoldPath|TestRun151954|TestIsFlowPlannerExcludedPathCoversSkillpackScaffold' -count=1` → green.
- `go test ./internal/runner -run 'TestFlowScopeDriftDetects|TestFlowScopeDriftBlocks|TestFlowCoderUsesFrozenScope|TestFlowCoderComputesWrittenPaths|TestFlowCoderEnforcedEvenWhenLiveTopologyUnresolved|TestFlowCoderBlocksOnUnobservableFrozenBaseline|TestBUG327|TestRun243681' -count=1` → green (security + legacy gate paths).
- `go test ./internal/changecontract/... -count=1` → green.
- `go vet ./internal/runner ./internal/changecontract` clean.

## Manual (operator tick)

Restart runner → re-run F3: the skillpack re-install no longer parks the coder
gate; a REAL out-of-scope file (e.g. `user.go` via the build-break recipe)
still parks, then amend via `POST .../agent-loop/amend` (CA-647) → retry pass.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-43
change_type: bugfix
summary: FrozenContractScopeDrift gate exempts tool-owned scaffold surfaces (.claude/** .agents/** .grok/** AGENTS.md CLAUDE.md .gitignore) via shared changecontract.IsToolOwnedScaffoldPath, mirroring the CA-645 freeze-guard fix, while keeping CA-427 .flowpilot/** security hole closed (gate rules rewrites still drift)
# --->8---