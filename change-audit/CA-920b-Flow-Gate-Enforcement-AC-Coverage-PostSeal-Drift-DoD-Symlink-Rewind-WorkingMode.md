---
id: CA-920b
title: Flow-gate enforcement — review AC coverage on HTTP/bridge, post-seal gate evaluation, runner-owned drift exemptions, Acceptance-Check DoD, symlink workspace, frozen-contract rewind guard, turn flowRef working mode (BUG-392..397, 400)
type: BugFix
feature: flow-gates
date: 2026-09-23
status: done
---

## Context

Live-verification wave found seven enforcement holes across CP-41/43/44/49/
55/58/61/62/64: review outcomes could bypass acceptance-criteria coverage on
the HTTP and bridge paths, post-seal follow-up turns skipped gate evaluation
entirely, runner/provider bookkeeping files were counted as scope drift, the
`r-dod-present` rule demanded a section the task-writing spec never emits,
declared paths under a symlinked workspace were rejected as symlink escapes,
an agent could `git checkout --` the frozen-contract ledger to erase it
before the lock step, and a turn-level `flowRef` bypassed working-mode
restrictions that run creation enforces.

## Changes

### BUG-392 — review AC coverage bypassed via HTTP flow-control and bridge

`internal/runner/review_ac_coverage.go` + `interactive_handlers.go`:
`turnBridge.SubmitFlowControl` now runs `validateReviewACCoverage` BEFORE
routing review outcomes, and the HTTP `handleSubmitFlowControl` path maps
review outcomes through `reviewOutcomeToFlowControl` with the same coverage
check. Coverage runs before the delegate-child guard so reviewer children
are checked while verdict-only/non-review paths keep their semantics.
Tests: `TestBug392_HTTPFlowControlEnforcesACCoverage`,
`TestReviewACCoverage_BridgeSubmit_RejectsBeforeProcessing`, plus 10
edge-case tests (newest-doc selection, sibling output template, owner
verdict-only bypass, no-doc passthrough, raw flow-control bypass, vibe hub
partial verdicts, cache, malformed args, zero-row read-only).

### BUG-393 — post-seal turns skipped gate evaluation

`internal/runner/interactive_service.go`: `turnStartedAfterLoopDone` no
longer suppresses post-turn gate evaluation. Admission semantics are
unchanged (blocked loops stay refused by the existing fence; done/stopped
loops still admit follow-up chat per BUG-302/308), and BUG-305's pin holds —
post-seal turns do NOT arm `pendingFlowGateSettle`. What changed: the gate
now still evaluates the turn's diff, so a post-seal write that violates
contract rules emits `flow_gate_violation` instead of sailing through.
Tests: `TestBug393_BlockedLoopStillRefused`,
`TestBug393_PostSealTurnDoesNotArmSettle`,
`TestBug393_GateEpochValidForPostSealTurn`,
`TestBug393_PostSealTurnStillGateEvaluated` (e2e: real git workspace,
adapter writes a file → r-ca/gate_blind violation observed).
Regression pin: `TestV10FlowEngineRootAfterLoopDoneCompletesImmediately`.

### BUG-394 — runner/provider bookkeeping counted as scope drift

`internal/changecontract/frozen_scope.go` + `internal/runner/gate_hook.go`:
new `IsRunnerOwnedConfigPath` exempts exact bookkeeping files
(`.flowpilot/guard/test_baseline.json`,
`.flowpilot/settings/gate-config.json`, `.devin/mcp_config.local.json`)
from frozen-contract scope-drift. Deliberately exact-match — the whole
`.flowpilot/**` tree stays covered, so `.flowpilot/settings/flow-rules.json`
or forged contract files still drift-flag (CA-427 boundary preserved).
Test: `TestBug394_RunnerOwnedGateBookkeepingPathsExempt`.

### BUG-395 — `r-dod-present` required a section the spec never emits

`internal/flowgate/dod.go` + `evaluate.go`: `## Acceptance Check` checklists
are accepted as DoD-equivalent alongside `## Definition of Done`, `## DoD`,
numbered and parenthesized forms. The parser keeps scanning after an
Acceptance-Check section so documents whose checklist lives under a later
heading still parse (union semantics; fenced code blocks stay ignored).
Tests: `bug395_dod_acceptance_check_test.go`; real-repo corpus test
`TestParseDefinitionOfDone_RecognizesRealRepoDocs` back to green.

### BUG-396 — declared paths false-positive under symlinked workspace

`internal/changecontract/paths.go`: `NormalizeDeclaredCodePaths` resolves
the workspace root itself (`filepath.EvalSymlinks`) before comparing, so
declared paths and the workspace live in the same coordinate system.
Absolute declared paths and not-yet-existing in-scope paths are handled;
real symlink escapes still reject. Side effect: the previously-failing
symlink-dependent `TestRun147126_*` baseline tests now pass under macOS
`/var→/private/var` temp dirs. Test: `bug396_symlink_workspace_test.go`.

### BUG-397 — agent `git checkout --` rewound the frozen-contract ledger

`internal/runner/contract_state_guard.go` (new) + `interactive_service.go`:
when an active frozen contract exists, destructive git commands targeting
`.flowpilot/**` (`checkout`/`restore`/`reset`/`clean`/`rm`/`switch`/`stash`
with flowpilot paths or bare-token forms) are denied at the approval-bridge
decision — before YOLO auto-approval and before the ordinary policy chain.
No active contract → no guard; non-destructive git unaffected. Live root
cause: cp64 run-5307 replayed `git checkout --
.flowpilot/contracts/frozen_contracts.ndjson` ×25, erasing the record before
the lock step. Tests: `TestBug397_GitRewindOfFlowPilotStateDenied`,
`TestBug397_SafeGitCommandsUnaffected`, `TestBug397_NoActiveContract_NoGuard`.

### BUG-400 — turn-level `flowRef` bypassed working-mode restrictions

`internal/runner/interactive_handlers.go`: `handleStartTurn` now applies
`FlowAllowedForWorkingMode` to a resolved `flowRef` before dispatch —
`vibe-sprint` under a `dev` run is rejected while allowed dev harness flows
pass. Run-creation enforcement is untouched. Tests:
`TestBug400_TurnFlowRefRespectsWorkingMode`,
`TestBug400_TurnFlowRefHarnessAllowedUnderDev`.

## Verification

- Focused: all Cluster E tests green (23 assertions across runner,
  flowgate, changecontract).
- Packages: `go test -count=1 ./internal/changecontract/...`
  `./internal/flowgate/...` green; `./internal/runner/` full suite — 22
  failures, 21 byte-identical on the clean baseline worktree
  (`/private/tmp/fp-baseline` @ d191004f) → pre-existing; the remaining
  `TestRun75035_SeedChildIgnoresSiblingCodexSessionPollution` is a
  process-wide env-var race (`FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH`
  resolved lazily at save time) — passes 3× in isolation on both trees.
- Provider parity: all changes sit in shared seams (gate hook, approval
  bridge decider, HTTP handlers, contract normalization) — provider-agnostic
  by construction; no adapter/event-stream/session code touched.

# ---8<--- flowpilot:change-ledger
feature_key: flow-gates
source_doc_id: BUG-392
change_type: bugfix
summary: Flow-gate enforcement — AC coverage, post-seal drift, DoD, symlink rewind, working mode
# --->8---
