# CA-1064 — r-scaffold-red honors a declared zero-red contract waiver

## Symptom (live run-15525 / gated child run-17384, vibe sprint)

The workspace seeded a pre-existing implementation (`src/tool.go` returning
`"debate-verify"`) plus a green baseline suite **before** the scaffold
phase. Owner-debate remediation adjudicated the anomaly correctly: the
frozen `requirements/.flowpilot/vibe/tdd-signatures.md` was rewritten to
record `red_tests: []` + `failure_type: none` + `Status: scaffold_ready` —
the pre-existing implementation is the accepted artifact and must NOT be
churned back into panic stubs (reverting it would break the green baseline
that `AC-1`–`AC-3` pin).

The post-turn gate nevertheless kept emitting the same `r-scaffold-red`
violation — "the suite passed, so the stubs contain real implementation —
revert every body to the stub whitelist" — on every re-completion. Owners
correctly escalated every round: no compliant coder-side action exists for
a gate demand that contradicts the adjudicated contract. The sprint could
never advance past `tdd`.

## Root cause

`checkScaffoldRedRule` (Signal 1 + Signal 3) treats an all-green suite and
non-stub bodies as smuggled implementation — correct for an ordinary
scaffold turn, but it never consulted the node's own declared RED-gate
expectation. The durable contract document already records the waiver in a
deterministic `key: value` section; the gate simply did not read it.

## Fix

- `TurnResult.ScaffoldRedWaived` (flowgate/rules.go): new signal field.
- `vibeScaffoldRedWaived(cwd)` (runner/vibe_sprint.go): deterministic parse
  of the declared contract doc — waived iff a `red_tests:` line carries an
  empty list literal `[]` AND a `failure_type:` line carries `none`
  (last declared value for each key wins). Both keys required; missing
  file, non-empty red_tests, or a real failure_type never waive.
- `gate_hook.go` child gate: `tr.ScaffoldRedWaived` is populated for
  scaffold turns alongside the existing static-body signals.
- `checkScaffoldRedRule` + `ScaffoldSatisfied` (flowgate/scaffold_red_rule.go):
  under the waiver the suite must still compile, run, and be **green** —
  a RED suite is now a violation in the opposite direction ("the accepted
  artifact is broken; fix the implementation, not the tests"). Signals 1
  and 3 no longer demand stubs.

Fail-closed posture preserved: an absent or partial declaration keeps the
original RED semantics; the waived path still requires fresh green suite
evidence every turn, and the pass path still runs
`recordScaffoldArtifactsLock` (read-only test lock + signature hash pin),
so the accepted artifact becomes the locked contract.

## Tests

- `internal/flowgate/scaffold_red_waiver_test.go` — green suite under the
  waiver is satisfied; RED suite, compile failure, and an absent suite run
  still violate.
- `internal/runner/vibe_scaffold_red_waiver_test.go` — parser accepts the
  live run-17384 doc shape; rejects normal scaffolds, missing files, and
  partial declarations.

## Provider parity

Provider-agnostic: the change reads a durable workspace contract artifact
and a gate `TurnResult` field — no adapter, event stream, or session code.
