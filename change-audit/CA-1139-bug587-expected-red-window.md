# CA-1139 — BUG-587: parent gate escalated a contractual TDD-red suite

## What changed

`apps/local-runner/internal/runner/gate_hook.go`:

- New `expectedRedSuiteWindowLocked`: a flow-driven parent is inside the
  expected-red window while a scaffold (`agent.scaffold`) or code-writer
  (`agent.code`) node's step is RUNNING / WAITING_USER_APPROVAL.
- In that window the parent turn suppresses the tier-2 test rules
  (r-tests/r-reg). The legs' own turns keep their dedicated gates
  (r-scaffold-red, r-signature-lock) — regressions are still adjudicated by
  the leg that owns the suite.
- Once the writing phase completes, the parent gates the suite normally.
- Skipped when the turn already carries the reproduce exemption.

## Live evidence (run-139670)

Parent turn-143575 ended while the tdd leg (run-143578) was mid-flight;
the gate ran the stub-red suite and fired r-tests/r-reg → owner-debate
escalation over an expected state.

## Tests

`bug587_expected_red_window_test.go` — red-first: window opens on scaffold
RUNNING, persists through coder RUNNING, closes when coder is DONE.
