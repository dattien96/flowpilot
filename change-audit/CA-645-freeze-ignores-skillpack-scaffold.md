# CA-645 — contract.freeze planner guard ignores skillpack/scaffold surfaces (run-151954 F2 false park)

## What

CP-43 F2 live (gate-sandbox, run-151954, rag-harness): flow parked at
`preflight_contract_freeze` WAITING_USER_APPROVAL with:

```
flow_contract_freeze_blocked: "planner changed 9 file(s)
(.claude/skills/gitnexus/gitnexus-cli/SKILL.md,
 .claude/skills/gitnexus/gitnexus-debugging/SKILL.md,
 .claude/skills/gitnexus/gitnexus-exploring/SKILL.md,
 .claude/skills/gitnexus/gitnexus-guide/SKILL.md,
 .claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md,
 .claude/skills/gitnexus/gitnexus-refactoring/SKILL.md,
 .gitignore, AGENTS.md, CLAUDE.md);
 the contract planner must be read-only"
```

The contract-planner never wrote anything.

## Why

`runContractFreezeNode` verifies the read-only planner by diffing the
flow-start worktree fingerprint against a fresh fingerprint at freeze time.
The 9 files were installed by the GitNexus **skillpack install** at
`06:58:50` — 17s AFTER the flow-start baseline snapshot (`06:58:33`) and 13s
before the freeze check (`06:59:03`) — i.e. mid-planner-turn tool-owned
scaffolding, not a planner mutation.

`isFlowPlannerExcludedPath` (added in CA-640 for run-243681) only excluded
`.flowpilot/**` and `.gitnexus/**`; it did not cover the skillpack surfaces
(`.claude/**`, `.agents/**`, `.grok/**`, `AGENTS.md`, `CLAUDE.md`,
`.gitignore`) → false-positive hard block on the freeze step.

## Fix

`apps/local-runner/internal/runner/flow_validate_audit_dispatch.go`:

- Extended `isFlowPlannerExcludedPath` to also exclude `.claude/**`,
  `.agents/**`, `.grok/**` (agent skill dirs written by skillpack/desktop
  skill sync) and the root scaffold files `AGENTS.md`, `CLAUDE.md`,
  `.gitignore` (skillpack install writes these alongside the skills).
- `baselineWorktreeFingerprint` and `worktreeMutatedSincePaths` inherit the
  exclusion automatically (both already call `isFlowPlannerExcludedPath`).

Real project files (`calc.go`, `user_test.go`, `planner_touched.go`) are
still captured and still block — the guard is not weakened for genuine
planner mutations.

Provider-agnostic (Case 1): no `ProviderKey` branch in the freeze dispatch.

Will not undo: CA-426 C-1 pre-existing-dirty allowance, CA-640 (`.flowpilot`/
`.gitnexus` exclusion), CA-634/635 frozen-scope drift, CA-637/639.

## Tests

Additive only — `flow_contract_freeze_test.go` and
`run243681_freeze_planner_metadata_exclusion_test.go` untouched.

- `apps/local-runner/internal/runner/run151954_skillpack_scaffold_exclusion_test.go` (new):
  - `TestRun151954_FreezeIgnoresSkillpackScaffoldMidFlowWrites` — exact repro:
    9 skillpack files appear between flow start and freeze → freeze succeeds,
    coder spawns.
  - `TestRun151954_FreezeStillCatchesPlannerProjectFileMutation` —
    `planner_touched.go` after flow start → still blocks, no coder spawn.
  - `TestIsFlowPlannerExcludedPathCoversSkillpackScaffold` — exclusion matrix
    (tool-owned excluded, project files kept).
  - `TestWorktreeMutatedSincePathsIgnoresSkillpackScaffold` / `...StillCatchesProjectFileAlongsideScaffold`
    — pure diff: scaffold churn ignored, real change still reported.
  - `TestBaselineWorktreeFingerprintExcludesSkillpackScaffold` — baseline skips
    scaffold, still captures `real.go`.

## Verification

- `go test ./internal/runner -run 'TestRun151954|TestIsFlowPlannerExcludedPathCoversSkillpackScaffold|TestWorktreeMutatedSincePaths|TestBaselineWorktreeFingerprint' -count=1` → green.
- `go test ./internal/runner -run 'TestRun243681|TestRunContractFreezeNode|TestInlineChain|TestRecoveredFlowReuses|TestDuplicateFreeze|TestFlowCoderUsesFrozenScope|TestFlowScopeDriftBlocks|TestFlowFrozen|TestFlowCoderComputesWrittenPaths|TestRun147126' -count=1` → green (legacy paths unaffected).
- `go vet ./internal/runner` clean.

## Manual (operator tick)

Restart runner (load fix) → Continue run-151954 (the 9 scaffold files are now
committed as `d348696`, so the freeze guard passes) → F2 proceeds to the coder
where the real `FrozenContractScopeDrift` hard-block can be exercised.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CA-645
change_type: bugfix
summary: contract.freeze planner-mutation guard also excludes skillpack/scaffold surfaces (.claude/** .agents/** .grok/** AGENTS.md CLAUDE.md .gitignore) so mid-flow skill installs stop false-blocking the freeze step (run-151954 F2 park, 9 files)
# --->8---