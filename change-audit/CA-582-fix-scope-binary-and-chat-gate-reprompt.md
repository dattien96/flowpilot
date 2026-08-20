# CA-582 — Fix r-scope binary noise and plain-chat gate stall

## What

Two runner fixes for CP-43 P-1/P-2 follow-up after run-208282:

### 1. r-scope must not flag build artifacts

- `apps/local-runner/internal/flowgate/observe.go:245` — `IsDocOrAuditFile` now also excludes build artifacts via new `IsBinaryOrBuildArtifact` (`*.exe/.dll/.so/.dylib/.a/.o/.out/.test/.bin`, `bin/` `dist/` `build/` `out/` `target/` `.tmp/` `tmp/`, bare root binaries like `gatesandbox`). Added `IsIgnoredForScope` wrapper.
- `apps/local-runner/internal/changecontract/scope.go:24` — `ScopeDiff` already uses `IsDocOrAuditFile`, so `gatesandbox` (and other binaries) no longer counts as `actual_touched \ declared_scope`. Docs (`requirements/` `change-audit/` `.flowpilot/` `*.md`) remain excluded as before.

### 2. Plain-chat gate must not stall at one auto-reprompt line

Root cause (run-208282 vs run-204658):
- Flow children/roots arm `pendingFlowGateSettle` before the post-turn gate, so `Completed` is deferred and `dispatch`'s `evaluateSettleGate` waits for the real gate (reprompt → `scheduleRootGateRepromptOrPark` → `startTurnClearingIntent`).
- Plain chat previously published `Completed` immediately (`emitLocked` `else` branch), so `dispatch` saw `Completed` at 16:26:16 and returned `allow:true` (finalized), while the real gate at 16:26:18 queued `pendingGateRepromptPrompt` — the UI showed `[GATE] auto-reprompt … gatesandbox; r-newtest; r-tamper` and then stalled with no turn 2.

- `apps/local-runner/internal/runner/interactive_service.go:4614` — `emitLocked` `EventTurnCompleted` for roots now defers `Completed` when the turn touched code (`EventFileChanged` not doc/binary) and `!turnStartedAfterLoopDone`. Plain turns with code arm `pendingFlowGateSettle` like flow roots; doc-only turns (e.g. plain "hi") still complete immediately, preserving the original 3-event shape. Added `flowgate` import for the code check.
- `apps/local-runner/internal/flowgate/observe.go` — binary check shared via `IsBinaryOrBuildArtifact`, so the `hasCode` test correctly treats `gatesandbox` as non-code.

## Tests

Additive only — no legacy suite edits:

- `apps/local-runner/internal/flowgate/binary_artifacts_test.go` (new, provider-agnostic):
  - `TestIsBinaryOrBuildArtifact` — 21 path matrix (gatesandbox, bin/app, extensions, known text files like Makefile/Dockerfile remain code).
  - `TestIsDocOrAuditFileExcludesBinary` — `IsDocOrAuditFile("gatesandbox")` true, `calc.go` false.
  - `TestIsIgnoredForScope` — CA/Task docs + binary ignored, `calc.go` not.
- `apps/local-runner/internal/changecontract/scope_binary_test.go` (new):
  - `TestScopeDiffIgnoresBinaryArtifacts` — declared `calc.go, calc_test.go` + diff containing `gatesandbox`, `bin/app`, CA/Task docs → `out` empty.
  - `TestScopeDiffStillFlagsRealOutOfScope` — `format.go` outside declared still flagged.
  - `TestScopeDiffIgnoresBinaryButNotCode` — `gatesandbox` ignored, `format.go` still flagged.
- `apps/local-runner/internal/runner/chat_gate_binary_and_reprompt_test.go` (new, × claude/codex/grok):
  - `TestBinaryScopeIgnoredForChatGate` — `gatesandbox`/`bin/app` ignored, `calc.go` not.
  - `TestPlainChatShouldDeferGateWhenCodeChanged` — `calc.go` considered code → should defer.
  - `TestPlainChatShouldNotDeferGateWhenOnlyDocs` — CA/Task docs not code → should not defer.
  - `TestPlainChatBinaryNotConsideredCode` — `gatesandbox` not code.
- Existing suites:
  - `go test ./internal/flowgate -run TestIsDoc` pass; `go test ./internal/changecontract -run TestScopeDiff` 9/9 pass.
  - `go test ./internal/runner -run TestNormalTurnPersistsWithSeq` now passes (plain "hi" without code still 3 events); `TestBinaryScopeIgnoredForChatGate` etc. pass.
  - `go test ./internal/tui/app` green (CA-580 intact).

## Provider impact

Provider-agnostic (Case 1): path classification and gate-arm logic do not branch on `providerKey`. New runner tests parameterize `claude`/`codex`/`grok` to prove parity.

## Verification

- `go vet ./internal/flowgate ./internal/runner ./internal/changecontract` clean (other than pre-existing `flow-pack` asset warnings).
- `go test ./internal/flowgate -count=1 -run TestIsBinary` pass.
- `go test ./internal/changecontract -count=1 -run TestScopeDiff` pass.
- `go test ./internal/runner -count=1 -run 'TestBinaryScopeIgnored|TestPlainChat|TestNormalTurnPersistsWithSeq' -v` pass (×3 providers).
- Full `go test ./internal/runner -run TestNormalTurnPersistsWithSeq` etc. — 5 pre-existing failures on clean tree remain (`TestEngineInitSkipsCurrentBindTrigger`, `TestFirebaseToolsMcpAdapterFetchEndToEnd`, `TestFirstCoderContextUsesCurrentFlowDeclaredPaths`, `TestFinalizePartialFailureCommitsNoHeadInTheBatch`, `TestSupabaseCatalogStoreShaping`) reproduced via `git stash --include-untracked` — not a regression.

## Residual

- `r-newtest`/`r-tamper` remain separate gates; C1 should use a NEW `*_test.go` file to pass them, or relax the verify to "no `r-contract` and declared paths correct" for P-1 close.
- Binary heuristic treats any root bare file without dot as binary except allowlisted text files (`Makefile`, `Dockerfile`, etc.); nested bare files without dot are conservatively not treated as binary.

# ---8<--- flowpilot:change-ledger
feature_key: change-contract
source_doc_id: CP-43
change_type: bugfix
summary: exclude build artifacts from r-scope and defer plain-chat gate so auto-reprompt actually starts a remediation turn
# --->8---
