# BUG-525 — Agent result text can override the recorded tournament patch

## Severity

High — untrusted agent prose can become the merge payload.

## Evidence

- `apps/local-runner/internal/runner/tournament_dispatch.go:270-290`
- `apps/local-runner/internal/runner/tournament_dispatch.go:384-415`
- `apps/local-runner/internal/runner/tournament_dispatch.go:529-558`

On the automatic arbiter `done` path, the triggering `resultMessage` is forwarded into `runTournamentMergeNode`. That function always calls `extractOperatorPatch(resultMessage)`. If a candidate or hub result naturally contains a `diff --git` block, it replaces the durable winner snapshot even though no operator submitted a merge resolution.

## Impact

The patch applied to the main workspace can differ from the patch the arbiter scored and selected. This bypasses the tournament snapshot boundary and lets agent-authored prose influence merge contents.

## Missing regression

Drive arbiter auto-pick with a terminal agent message containing a different valid unified diff. Assert that the recorded winner snapshot remains the only merge source. Separately keep the HTTP decision-card path proving that an authenticated operator option plus diff can override it.

## Scope

Capture only. No fix applied during the 2026-09-27 branch review.
