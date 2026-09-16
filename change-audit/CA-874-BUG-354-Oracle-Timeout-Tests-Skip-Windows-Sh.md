# CA-874 — BUG-354 oracle-timeout tests: skip unix-only .sh fixtures on Windows

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-354
change_type: test
summary: skip the two .sh-executing oracle-timeout tests on Windows, matching the existing setsid test guard in the same file
# --->8---

## Why

`Test540927OracleSigtermIgnoringSuiteBounded` and
`Test540927OracleCleanSuiteStillPasses`
(`internal/flowgate/run540927_oracle_timeout_test.go`) exec `.sh` fixture
scripts directly. Windows cannot execute `.sh` (`%1 is not a valid Win32
application`), so both failed on Windows dev machines while passing on Linux
CI. No production defect — the oracle timeout/kill-grace plumbing works; only
the fixture vehicle is unix-only. Operator explicitly authorized resolving
these (test-only change, no production code touched).

## Change

- Added the file's own established guard to both tests:
  `if runtime.GOOS == "windows" { t.Skip(...) }` — identical to the
  pre-existing guard in `Test540927OracleGraceAbandonWhenKillMissesPipeHolder`
  (`setsid escape is unix-only`). Coverage is preserved on Linux CI; nothing
  was deleted. Chose skip-over-delete so the BUG-354 kill-grace pins keep
  running where they can execute.

## Tests

- `go test ./internal/flowgate/ -count=1` → full package green on Windows
  (the 2 tests SKIP, the rest PASS, including the other run540927 tests).
- No production files touched; no provider surface involved (flowgate oracle
  is provider-agnostic); no other test modified.

## Prior CA claims kept intact

- BUG-354 (CA-742/743): oracle-hang bounds, kill grace, EnvError-on-timeout
  semantics unchanged — skips are platform gates only, assertions identical.
