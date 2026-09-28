# CA-631 — Quota Answer Rollback on Apply Failure (BUG-541)

**Date**: 2026-09-28
**Author**: Devin
**Ticket**: BUG-541

---

## Problem

`AnswerQuestion` marked the durable question `resolved` at HTTP-admission
time, then ran `applyQuotaRouteAnswer` and log-swallowed its failure. When
the quota-route apply failed (live: `spawnChildRun` correctly refused a
respawn while the parent loop was `blocked(escalate)` — BUG-432 guard), the
card was consumed and unanswerable forever: parent stranded in
`waiting_question` with no pending card, candidate never respawned, child
left a `running`+`leg:closed` zombie.

## Fix

### `internal/runner/quota_gate.go`

`applyQuotaRouteAnswer` returns `*apiErr`; failure propagates instead of
being logged and dropped.

### `internal/runner/interactive_service.go` (`AnswerQuestion`)

On apply failure the question rolls back to `pending` and is re-mirrored —
the card remains actionable. The HTTP caller still receives `accepted`
(answer recorded), but the durable record stays answerable so the apply can
be retried when the loop unblocks.

## Tests

`bug541_quota_answer_apply_failed_test.go`:

- `TestBug541_CardSurvivesApplyFailure` — card rolls back to pending when
  the respawn is refused on a blocked parent.
- `TestBug541_ApplyFailureReturnsError` — the apply error surfaces to the
  caller.
