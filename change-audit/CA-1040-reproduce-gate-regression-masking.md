# CA-1040 — Reproduce gate regression masking (BUG-531)

## Summary

Live deferred-case drilling on devin/swe-2-high found the reproduce gate
reporting "the suite passed, so the bug was not reproduced" while its own
oracle observed a red suite (`go test -v .` exit 1). The false verdict
burned the bounded reprompt budget and escalated the flow, so a real bug
that also breaks a baseline-green test could never satisfy r-reproduce.

## Changes

- `internal/runner/gate_hook.go` (both gate entry paths): for a reproduce
  turn, `tr.Tests.Failed` is repopulated from `oracle.Failed` — every named
  test that failed this run, override-filtered — instead of only the
  non-regression split. A regression of a baseline-green test no longer
  masks the reproduction.
- `internal/runner/bug531_reproduce_gate_regression_masked_test.go`: new
  additive regression test modelling the live shape (green baseline,
  uncommitted bug introduction, red repro test + red baseline test).
- `requirements/09-BugFix/todo/BUG-531-*.md`: capture doc.

## Red test

Before the production change, `TestBug531_ReproduceGatePassesWhenBaselineTestAlsoRegresses`
failed by assertion: the gate reprompted (blocked=true) while the oracle
observed exit 1 with two named failures.

## Live verification (run-1481, devin/swe-2-high, /tmp/fp-live2)

- bug-harness on a green repo; reproducer correctly asked the
  green-on-arrival question (`user_question_required` q-2014).
- Operator re-seeded the bug (`base.go` -> `return 0`) and answered;
  reproducer retry produced a genuinely red suite.
- With the fix: reproduce gate PASSED (previously it reprompted twice and
  escalated at the cap — the pre-fix live behavior on run-134).
- Flow completed end-to-end: reproduce -> implement -> validate ->
  reviewer -> synthesis -> audit, run `completed`. `go test ./...` green
  in the live bed; the repro test file was locked read-only and kept as
  evidence.

## Provider parity

Runner-core gate semantics — provider-agnostic (same TurnResult/oracle path
for every adapter). Verified live on devin/swe-2-high only; no Claude/Codex
accounts on this machine.

## Follow-ups observed live (not fixed here)

- Freeze escalate on an unparseable planner draft parks with no option card;
  operator must supply a corrected draft via continue (worked, but discoverable
  only from logs). Worth a BUG if it recurs.
- Audit `blocked_missing_feature_key` requires a workspace
  `change-audit/FEATURE-KEYS.md` registry entry — resolved by operator adding
  the key, then audit passed. Gate behaved fail-closed as designed.
