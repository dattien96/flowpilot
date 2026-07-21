# BUG-304 — Hub-reinvoke test races the deliberate pendingAgentContext removal

## Metadata

- Document ID: `BUG-304`
- Title: `TestNonCohortPreflightStartTurnFailReinvokesHub` races the deliberate `pendingAgentContext` removal
- Phase: `bugfix`
- Status: `done`
- Owner: local-runner (test-only)
- Reviewers: n/a
- Created: 2026-07-21
- Last Updated: 2026-07-21
- Parent Documents: BUG-275 (join-note duplicate-copy removal, the mechanism this test was racing against), BUG-303 (companion — found alongside this one during the same full-suite verification pass)
- Child Documents: none
- Related Documents: none
- Replaces: none
- Tags: agent-flow-engine, test-flake

## AI Quick View

### Summary

- Found while running a full-suite regression baseline to verify BUG-302 — `TestNonCohortPreflightStartTurnFailReinvokesHub` failed intermittently (~30% of runs), unrelated to BUG-302 itself.
- **Not a production bug.** The failure note IS always correctly delivered to the hub's reinvoked turn — `maybeAutoReinvokeHubWithNote` embeds it directly in the reinvoke prompt. As a deliberate anti-duplication step (BUG-275), it then also removes that same note from `pendingAgentContext`, so `startTurn`'s own context-block composer doesn't wrap and re-send a second copy.
- The test asserted on `pendingAgentContext` synchronously, immediately after the synchronous append — racing against the background reinvoke goroutine's own (correct, intentional) removal of the exact same note. Whichever goroutine the scheduler ran first decided pass/fail; the note was delivered correctly either way.

### Current Ask

- Make the test assert on something that cannot race: the content actually delivered to the provider, not the transient `pendingAgentContext` field that is deliberately drained/pruned as part of correct delivery.

### Key Decisions

- `V-1` Capture the prompt the fake adapter actually receives (via a mutex-guarded variable in the test's `newAdapter` closure) and assert on that, polled until non-empty — this only becomes true once the reinvoke has genuinely reached the provider, so there is no race window left to hit.

### Constraints

- This is a test-only change (`additive-tests-only` §"if a pre-existing test must be changed... stop and ask" — asked, and the user approved fixing it in this same request). No production code changed for this bug.
- Must keep verifying the test's original intent: the failure note reaches the reinvoked hub turn.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/run1618_entry_fail_hub_hang_test.go` (`TestNonCohortPreflightStartTurnFailReinvokesHub`)
- `apps/local-runner/internal/runner/interactive_service.go` (`notifyHubOfFlowChildFailureLocked`, `maybeAutoReinvokeHubWithNote`)

## 1. Issue Summary

While capturing a full-suite regression baseline (unrelated to the change being verified), `TestNonCohortPreflightStartTurnFailReinvokesHub` was observed to fail intermittently. Re-running it in isolation both with and without an unrelated in-flight change (BUG-302) showed the SAME ~25–35% failure rate either way, ruling out that change as the cause and confirming a pre-existing flake.

## 2. Parent Links

- impacted coding plan: n/a (test infrastructure only)
- impacted tech design: n/a
- impacted system spec: n/a

## 3. Environment and Reproduction

- environment: pure Go goroutine-scheduling race, no OS/provider dependency; reproduces on any machine
- reproduction: `go test ./internal/runner -run TestNonCohortPreflightStartTurnFailReinvokesHub -count=8` on the pre-fix test — fails roughly 1 in 3 runs
- frequency: intermittent, scheduler-dependent

## 4. Expected vs Actual

- expected: the test deterministically confirms the failure note reaches the hub reinvoke.
- actual: the test's own synchronous read of `pendingAgentContext` non-deterministically raced a background goroutine that (correctly) removes the same note from that field once it has been embedded directly in the reinvoke prompt.

## 5. Impact

- users affected: none (no production behavior is wrong)
- workflows affected: CI/local test-suite reliability only
- severity: low (pure test flake — occasionally reported a false failure for otherwise-correct behavior)

## 6. Root Cause

- hypothesis: the test asserts on a field that production code intentionally mutates asynchronously as part of correct operation, without synchronizing against that mutation.
- confirmed cause: `handleChildStartTurnFailure` → `notifyHubOfFlowChildFailureLocked` synchronously appends the failure note to `pendingAgentContext`, then spawns `go s.maybeAutoReinvokeHubWithNote(parentRunID, failNote)`. That goroutine embeds `cohortNote` directly into the reinvoke prompt (`apps/local-runner/internal/runner/interactive_service.go:2089-2091`) and, per BUG-275, removes one copy of that same note from `pendingAgentContext` (`interactive_service.go:2073-2075`) so `startTurn`'s context-block composer doesn't ALSO wrap and resend it — correct, intentional anti-duplication. The test's own subsequent `svc.mu.Lock()` read of `pendingAgentContext`, executed immediately after the synchronous append returns, races that background removal for the same mutex with no ordering guarantee either way.
- evidence: re-ran the test 8× in isolation against both the unfixed and (unrelated) BUG-302-fixed code — 5/8 and 6/8 passes respectively, a statistically indistinguishable flake rate, confirming the race is intrinsic to the test itself and unrelated to any in-flight change.

## 7. Fix Strategy

- `F-1` Capture the prompt actually delivered to the fake `ProviderRuntimeAdapter` (the reinvoke's real destination) in a mutex-guarded test-local variable, and poll for it to become non-empty (bounded by the test's existing 2-second deadline) instead of reading `pendingAgentContext` synchronously right after the append. This assertion point cannot race: it is only ever satisfied after the reinvoke has genuinely reached the provider, at which point the note (embedded directly in the prompt) is unconditionally present.

## 8. Validation

- `V-1` **Not provider-specific:** the race is in test harness synchronization (a Go mutex/goroutine ordering issue), not in any provider adapter's behavior — the fake adapter here is a generic `ProviderRuntimeAdapter`, exercised identically regardless of which real provider it stands in for.
- `V-2` `go test ./internal/runner -run TestNonCohortPreflightStartTurnFailReinvokesHub -count=15` — 15/15 pass with the fix (previously ~1-in-3 failed over repeated runs).
- `go build ./...` passes.

## 9. Regression Guard

- tests: `TestNonCohortPreflightStartTurnFailReinvokesHub` itself, now deterministic.
- alerts: n/a
- audit checks: none.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: `notifyHubOfFlowChildFailureLocked`'s and `maybeAutoReinvokeHubWithNote`'s production behavior (including the BUG-275 note-removal this test was racing against) is correct and untouched — this fix only changes what the test observes and when.
