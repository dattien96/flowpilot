# CA-893 — CP-64 M-2: reproduce gate misclassified Go parse errors as "suite passed"

# ---8<--- flowpilot:change-ledger
feature_key: reproduce-first-gate
source_doc_id: CP-64
change_type: bugfix
summary: ClassifySuiteOutput now recognises the `[setup failed]` tag Go emits for load-phase parse errors (missing ',', unbalanced braces, etc.), so compile-failed suites take the "failed to compile" reprompt branch instead of the false "suite passed" branch; additive probe-test matrix added
# --->8---

## Why

CP-64 M-2 live verification on Grok-4.5 (`bug-harness` flow, workspace
`/private/tmp/gate-sandbox-verify`, run-593 / reproducer child run-706)
reproduced a real production bug: the seeded `textutil/title_repro_test.go`
contained a missing-comma parse error, `go test -v ./textutil/...` exited 1
with `FAIL\t<textutil> [setup failed]`, but the gate reprompt told the agent
"the suite passed, so the bug was not reproduced — add an assertion that
fails".

Root cause: `internal/flowgate/oracle.go` `classifySuiteOutput` (via
`ClassifySuiteOutput`) only recognised `[build failed]` (type-check errors)
and a few other markers. Go emits `[setup failed]` for load-phase parse
errors — a distinct tag that was missing, so `ReproduceCompileFailed` stayed
false and the reprompt fell into the green-suite wording.

Pre-fix live evidence (old binary :18760, real gate-sandbox workspace): the
same compile failure produced the "suite passed" reprompt. Post-fix live
evidence (rebuilt runner :18999): `[gate] suite end ... err=exit status 1
outputBytes=684` → reprompt "the reproduce test failed to compile — a
compile error is not a reproduction; fix the test's syntax/imports so it
builds and then fails on its assertion."

## Change

- `internal/flowgate/oracle.go`: added `[setup failed]` to the compile/build
  failure signatures in `classifySuiteOutput`.
- `internal/flowgate/classify_probe_test.go`: additive probe tests covering
  the observed output-shape matrix — missing-comma `[setup failed]`,
  unbalanced-brace `[setup failed]`, `[build failed]` type errors, clean
  assertion FAIL (must NOT classify as compile), and clean PASS.

## Verification

- `go test ./internal/flowgate/...` — green.
- Focused runner gate tests green; the two runner-package failures observed
  (`scaffold_lock_test.go`-related) are pre-existing on this branch and fail
  identically without this change.
- Live: post-fix Grok-4.5 run confirmed the corrected reprompt wording
  (evidence above, runner log :18999 2026-09-20 20:16).
