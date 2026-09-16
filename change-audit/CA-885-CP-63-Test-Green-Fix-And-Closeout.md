# CA-885 — CP-63 test-green fix + doc closeout (CP63-66 audit follow-up)

# ---8<--- flowpilot:change-ledger
feature_key: lsp-runtime
source_doc_id: CP-63
change_type: fix
summary: fix 3 red lsp tests at clean HEAD (unix fake-gradlew echo, Stop reap assertion) + 5 new matrix tests; move CP-63 + Task-354-363 todo to done with DOD ticked
# --->8---

## Why

The CP63-66 review (2026-09-16) found CP-64/65/66 genuinely done but CP-63
red: `go test ./internal/lsp/...` failed 3 tests at clean HEAD, contradicting
the CA-871 "green" claim, and CP-63 + Task-354→363 still sat in `todo/` with
the CP-63 DOD unticked. Failing tests are never "fixed" by weakening
assertions (oracle-rule) — both root causes below are test-side bugs with the
production code proven correct, corrected minimally with operator approval.

## Change

- **`internal/lsp/gradle_fallback_test.go` (`lspFakeGradlew`, unix branch)**:
  raw body lines are now emitted via shared `lspEchoLines` (printf-quoted).
  Before, an `e: ...` line executed as a shell command (`e:: command not
  found`, proven via `sh -c`) and a `> Task ...` line redirected stdout —
  stdout came back empty so the parser (correctly) found zero errors.
- **`internal/lsp/gradle_callsite_test.go` (`lspAndroidHarness`, unix branch)**:
  same echo fix. Windows `.bat` branches already echoed (untouched).
- **`internal/lsp/server_manager_test.go`** (2 minimal touches):
  `TestServerManagerStopKillsProcess` now asserts reaping only
  (`ProcessState != nil`); the old `Exited()` assertion can never hold on
  Unix because `Stop()` SIGKILLs by design and Go's `os.ProcessState.Exited`
  reports false for signal deaths (spec-cited, not an assumption).
  Added additive `hangkill` helper mode (EOF-proof; signal-only death).
- **New `internal/lsp/gradle_fake_echo_test.go`** (`lspEchoLines` home):
  fake round-trips redirect-looking/`$`/backtick/quote lines verbatim with
  no stray `Task` file; garbage-surrounded error parses to exactly 1;
  warning-only output parses to 0.
- **New `internal/lsp/server_manager_reap_test.go`**: kill shape reaped +
  platform `Exited()` semantics locked (`!Exited && !Success` on Unix,
  `Exited` on Windows); already-exited server still reaped by Stop.
- **Docs (closeout, no behavior)**: `git mv` CP-63 + Task-354→363
  `todo/` → `done/`; parent/child/related links retargeted to `done/`
  (SS-14/SS-19/CP-64 inbound links left as-is, matching the CP-64/65/66
  move precedent); CP-63 metadata → `done` + DOD §10 ticked with evidence.

## Tests

- 5/5 new tests green (`TestFakeGradlewRoundTripsSpecialLines`,
  `TestFakeGradlewEndToEndIgnoresGarbage`,
  `TestFakeGradlewWarningOnlyYieldsNoErrors`,
  `TestServerManagerStopReapsKill`, `TestServerManagerStopReapsExitedServer`).
- Previously-red 3 now green with NO assertion-weakening (helpers echo;
  reap-only assertion).
- Full `internal/lsp` package green (42s); blast-radius green: runner
  lsp/knowledge/profile suites, `cli`, `agentpack`, `flowgate`,
  `changecontract`.
- R1: zero production edits; 3 legacy test-helper/assertion lines corrected
  with explicit operator approval ("làm đi" after the failure report);
  everything else in the legacy suite untouched and green.
- R2: Case-1 agnostic — grep `providerKey|ProviderKey` over
  `internal/lsp/` = 0 hits; diagnostics flow through the shared post-turn
  gate path (CA-869 evidence stands).
- R3 matrix: reported repro (2 gradle + 1 reap), near-miss (redirect line,
  quotes/metachars, garbage-surrounded error, exited-server reap),
  degraded (warning-only, missing gradlew via pre-existing test),
  providers (agnostic proof above).

## Prior CA claims kept intact

- CA-869/870/871: gate allow-path contract, warn-once semantics,
  `CheckFiles` ""-on-degradation, `AfterFileWrite` delegation — no
  production file touched, all re-green. The CA-871 "green" claim is
  superseded for the 3 tests (they were red on Unix at clean HEAD).
- CP-64 (CA-872) / CP-65 (CA-876→880) / CP-66 (CA-881→884): untouched,
  suites re-green in the blast-radius sweep.
