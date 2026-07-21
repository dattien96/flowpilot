# CA-384 — Hub-reinvoke test race fix

## Summary

Fixed BUG-304, found while running a full-suite regression baseline to verify
an unrelated change (BUG-302): `TestNonCohortPreflightStartTurnFailReinvokesHub`
failed intermittently (~1 in 3 runs). Root-caused as a test-only race, not a
production bug — the failure note is always correctly delivered to the
reinvoked hub turn (embedded directly in its prompt); it is also, correctly
and deliberately (BUG-275), removed from `pendingAgentContext` right after,
to avoid a duplicate copy being wrapped in by `startTurn`'s own context-block
composer. The test read `pendingAgentContext` synchronously, racing that
background removal.

Fixed by capturing the prompt the fake adapter actually receives and polling
for it, instead of reading the transient `pendingAgentContext` field —
eliminates the race entirely (that assertion point is only reachable after
the note has genuinely reached the provider).

## Cross-provider parity

Not applicable in the usual sense — the race lives in test-harness
goroutine/mutex ordering, not in any provider adapter's behavior. The fake
adapter here stands in generically for whichever provider a real run would
use; the fix does not depend on which one.

## additive-tests-only compliance

This is a test-only fix to a pre-existing test. Per the skill's own
"if a pre-existing test must be changed... stop and ask" clause: asked, user
approved fixing it in the same request that reported the flake. No production
code changed for this bug.

## Verification

- `go test ./internal/runner -run TestNonCohortPreflightStartTurnFailReinvokesHub -count=15` — 15/15 pass (previously ~1-in-3 failed across repeated runs, confirmed via 8x-each-way isolation runs against both the unfixed and BUG-302-fixed code, ruling out that unrelated change as the cause).
- `go build ./...` passes.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-304
change_type: test
summary: Fix a test-only race in TestNonCohortPreflightStartTurnFailReinvokesHub by asserting on the delivered reinvoke prompt instead of the transient pendingAgentContext field that production code deliberately drains as part of correct, non-duplicated delivery.
# --->8---
