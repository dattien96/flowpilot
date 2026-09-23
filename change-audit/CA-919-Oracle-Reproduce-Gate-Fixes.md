---
id: CA-919
title: Oracle/reproduce gates — subtest parse, tamper propagation, lock paths, signature-record selection, reprompt cap, scaffold-RED, fabricated-RED (BUG-386..391, 398)
type: BugFix
feature: flow-gates
date: 2026-09-23
status: done
---

## Context

Live-verification wave found the oracle/reproduce/scaffold gate family broken
in seven independent ways across runs 169, 442, 2290, 4014, 4501, 5307, 6008,
19151.

## Changes

### BUG-390 — indented subtest lines parsed to junk name `"---"`

`internal/flowgate/oracle.go`: extracted the named-test scan into
`parseSuiteTestNames(testCmd, output)`; `--- PASS:`/`--- FAIL:` markers are now
located with `strings.Index` (indent-tolerant) instead of `Contains` +
`TrimPrefix`, which left the indent and recorded the literal name `"---"`.
`isInBaseline` now also rejects `""`/`"---"` on both sides so baselines already
polluted with junk entries cannot mark real failures as regressions. This was
the root cause of the reproduce-gate deadlocks: `HasRegression` suppressed
`tr.Tests.Failed`, so r-reproduce reported "the suite passed" on a RED suite.

### BUG-387 — `oracle.Tampered` dropped by the validate path

`internal/runner/flow_validate_audit_dispatch.go`:
`runValidateWithOracleIfPossible` now forces `ExitCode=1` and prepends a
`pre-existing test file(s) tampered:` detail when `oracle.Tampered` is
non-empty — the signal reaches `AdvanceRetryState` and the persisted
validation record instead of being discarded.

### BUG-388 — reproduce lock absolute-vs-relative bypass

`internal/changecontract/frozen_scope.go`:
`LockReproduceTestPaths`/`LockScaffoldArtifactsForStep` relativize stored
paths to the workspace (`relativizePathsToWorkspace`) so `ReadOnlyPaths` holds
the same shape the approval bridge's candidate does. New
`IsReadOnlyLockedPathUnder(rec, candidate, workspace)` additionally tolerates
legacy records that already hold absolute paths (in-flight runs must not
silently unlock). Runner call sites (`reproduce_gate.go` deny bridge,
`gate_hook.go` drift filter) use the workspace-aware variant.

### BUG-386 — signature lock never armed

`internal/runner/flow_validate_audit_dispatch.go`: `frozenContractForRun`
(topology scan AND the `ListForRun` restart fallback) now prefers the active
record carrying `SignatureHash`, falling back to the first active record. The
scaffold node's contract (no hash) no longer shadows the coder's locked
record → `coderSignaturesLocked` arms, `r-signature-lock` evaluates, and
`isSignatureLockedCoderChild` advertises `renegotiate_signatures`.

### BUG-391 — reprompt cap never reached

`internal/runner/interactive_service.go` + `interactive_resume.go` +
`gate_hook.go`: `startTurn` no longer zeroes `rs.repromptAttempts` when the
turn is a gate-reprompt delivery (`scenarioGateReprompt`, set by
`startTurnClearingIntent` for `kind=="reprompt"`). The counter therefore
accumulates across the reprompt chain and `maxFlowGateReprompts` actually
bounds the loop; a passed gate resets the counter so a later, unrelated
violation gets a fresh budget (fail-closed escalation preserved).

### BUG-398 — r-reg treats contracted TDD-RED as violation

`internal/flowgate/evaluate.go`: `tests_failed`/`regression_test_broke`
return nil when `tr.ScaffoldExpected` — the scaffold turn's mandated RED suite
is evaluated solely by `r-scaffold-red`.

### BUG-389 — fabricated RED accepted

`internal/flowgate/rules.go`: new caller-computed fields
`ReproduceTargetChecked`/`ReproduceExercisesTarget`.
`internal/flowgate/reproduce_rule.go`: `checkReproduceRule` reprompts when
Checked && !Exercises ("the failure looks fabricated").
`internal/runner/reproduce_gate.go`: `reproduceFailuresExerciseTarget`
resolves each failing test name to its `*_test.go` source (this turn's writes
+ `*_test.go` under declared dirs) and verifies it calls a symbol declared in
the step's frozen `DeclaredPaths` (Go targets; other languages →
`Checked=false` typed degradation). Wired into both root and child gate hooks.

## Tests

- `internal/flowgate/bug390_subtest_parse_test.go` (3 tests)
- `internal/flowgate/bug389_reproduce_target_test.go` (3 tests)
- `internal/flowgate/bug398_scaffold_red_suppression_test.go` (2 tests)
- `internal/changecontract/bug388_reproduce_lock_abspath_test.go` (2 tests)
- `internal/runner/bug386_frozen_contract_preference_test.go`
- `internal/runner/bug387_validate_tamper_test.go`
- `internal/runner/bug391_reprompt_cap_test.go` (2 tests)
- `internal/runner/bug389_target_resolution_test.go` (3 tests)

## Verification

- `go test ./internal/flowgate ./internal/changecontract` — green.
- `go test ./internal/runner` — the failure set is identical to the clean-HEAD
  baseline worktree (`/private/tmp/fp-baseline`, detached d191004f): all
  observed failures are pre-existing/flaky (TempDir RemoveAll races), none
  introduced by this change.

## Provider parity

All changes are in shared flowgate/changecontract/runner gate code — below the
provider seam; identical behavior for every adapter.

# ---8<--- flowpilot:change-ledger
feature_key: flow-gates
source_doc_id: BUG-386
change_type: bugfix
summary: Oracle and reproduce gate fixes
# --->8---
