# BUG-661 — Long-RetryAfter `rate_limited` escapes quota gate → run terminal-fails instead of park/rotate

## Metadata

- Document ID: `BUG-661`
- Title: `Long-RetryAfter rate_limited escapes quota gate — run terminal-fails instead of park/rotate`
- Phase: `bugfix`
- Status: `done`
- Fixed By: `CA-1080`
- Owner: `flowpilot runner`
- Reviewers: `dev`
- Created: `2026-10-09`
- Last Updated: `2026-10-09`
- Parent Documents: `CP-87` (quota routing), Task-445 (in-turn rate-limit retry), Task-449 T-4 (post-failure quota gate), Task-447 (account block ledger)
- Child Documents: `—`
- Related Documents: `run-616108` (PrivateVault vibe-tasks CP-12 live run)
- Replaces: `—`
- Tags: `provider-limit, rate-limit, quota-gate, run-lifecycle, severity-high`

## AI Quick View

### Summary

- A `rate_limited` provider failure whose `RetryAfterSeconds` exceeds the Task-445 in-turn retry bound (30s) terminal-fails the whole run — it never enters the post-failure quota routing gate because `finishTurn`'s hard-kinds switch excludes `rate_limited` unconditionally.
- The exclusion was designed for *transient* limits the send-loop retries — but nothing bounds it: a 37-minute Devin rate limit killed a live vibe-sprint run mid-implementation with no park, no account rotation, no escalation surface.
- The same exclusion shape repeats in `resolveQuotaGate` (ObservedLimit mapping) and `noteAccountBlockedLocked` (durable account block), so even if the gate fired, the just-failed account could be re-picked.

### Current Ask

- Route `rate_limited` failures that reach `finishTurn` into the quota gate — at that point in-turn retry is already exhausted or inapplicable, so the limit is terminal-for-now by definition. Carry it as `ObservedLimit`, and mark the account blocked so rotation cannot re-pick it during the reset window.

### Key Decisions

- `V-1` Any limit kind reaching `finishTurn` after `isRecoverableSendError` returned false is terminal-for-now — the gate exclusion must not re-assume "transient" a second time.
- `V-2` `rate_limited` blocked-account semantics = "unusable until window resets" — same shape as `quota_exhausted` (lifted on fresh headroom telemetry), not billing-style permanent.

### Constraints

- Do not regress Task-445: `providerLimitRecoverable` (≤30s + RetryAfter>0) must still be retried in-turn without ever reaching the gate.
- A transient rate limit that *survives all send attempts* must also reach the gate — exhaustion of in-turn retries is itself a quota signal.
- Resume path (`resumeRun` → heal + redrive quiet loop) already reconstructs the run; the fix is about not dying in the first place.

### Open Questions

- Should `noteAccountBlockedLocked` carry the `RetryAfterSeconds` value so rotation can un-block on schedule rather than only on headroom telemetry? (Follow-up; minimal fix uses quota_exhausted-style telemetry lift.)

### Source Refs

- `run-616108` — PrivateVault vibe-tasks CP-12; failed 2026-10-08T18:22:59Z.
- Events `evt-629548..550`: `turn_started` → `provider_limit_reached` (`kind=rate_limited`, `devin`, "reset in 37 minutes at 19:01 UTC") → `turn_failed`.
- Attention `settle_pending` (turn-629153, rev 4) was in-flight when the run died — run terminal-failed while terminal bookkeeping still settling.

## 1. Issue Summary

During a live `vibe-tasks` run (CP-12, Task-121 implementation — fully implemented and committed in-worktree), a Devin provider turn hit the free-model rate limit with a ~37-minute reset window. The turn failed terminally (correct: RetryAfter ≈ 2220s ≫ 30s in-turn bound). The run then went straight to `status=failed` — the post-failure quota routing gate (`enterQuotaGate`) was never invoked because `finishTurn` only routes the three "hard" kinds (`quota_exhausted`, `credits_exhausted`, `billing_required`). `rate_limited` is silently excluded on the assumption that Task-445's send-loop retry owns it — an assumption only valid for RetryAfter ≤ 30s.

## 2. Parent Links

- impacted coding plan: `CP-87` (quota-aware routing)
- impacted tech design: Task-445 T-3 (typed rate-limit in-turn retry), Task-449 T-4 (post-failure quota gate), Task-447 (durable account block ledger)
- impacted system spec: provider-limit taxonomy `provider_event.go` / `provider_limit.go`

## 3. Environment and Reproduction

- environment: local-runner dev @ `eefe8f95`, provider `devin` free model (`swe-2-high`), project PrivateVault, worktree `cht_fae8b7926e42`
- reproduction steps:
  1. Run a long multi-leg flow on a provider account near its free-tier rate limit.
  2. A mid-flow turn receives `rate_limited` with RetryAfter > 30s (e.g. "reset in 37 minutes").
  3. `isRecoverableSendError` → false → `turn_failed` → `finishTurn` hard-kinds switch skips `rate_limited` → no quota gate → `commitFinishTurnDispatchTerminal` → run `failed`.
- frequency: any provider rate limit with a reset window longer than 30s; guaranteed kill on free-tier exhaustion windows.

## 4. Expected vs Actual

- expected: a rate limit that outlives in-turn retry parks the run / enters the quota routing gate → account rotation, wait-for-reset, or a user decision card — never silent terminal failure of a multi-hour workflow.
- actual: `run-616108` marked `status=failed` with zero escalation surface; user must notice, wait ~37 min, and manually resume.

## 5. Impact

- users affected: anyone running long vibe-sprint/vibe-tasks flows on rate-limited provider tiers.
- workflows affected: all flow-driven runs (vibe-tasks, vibe-sprint, debate cohorts) — a single transient provider window kills the entire loop.
- severity: HIGH — loses in-flight loop progress surface (though commits are durable in-worktree); violates "never claim/allow unrecoverable termination without escalation".

## 6. Root Cause

- hypothesis: `finishTurn`'s quota-gate trigger switch treats all `rate_limited` as transient.
- confirmed cause: `interactive_service.go` ~line 11632 — `switch limit.Kind { case ProviderLimitQuotaExhausted, ProviderLimitCreditsExhausted, ProviderLimitBillingRequired: quotaGateRunID = rs.id }` — `ProviderLimitRateLimited` absent → `quotaGateRunID` stays "" → `enterQuotaGate` never called.
- evidence:
  - `provider_limit.go:106-112` — `providerLimitRecoverable` bounds in-turn retry to `Kind==rate_limited && 0 < RetryAfterSeconds <= 30`.
  - `interactive_service.go` finishTurn — the gate exclusion exists *after* that predicate already returned false; i.e. the "transient" assumption is applied twice: once correctly (don't retry), once incorrectly (don't escalate).
  - `quota_gate.go:resolveQuotaGate` — `ObservedLimit` mapping has the same hard-kinds-only switch.
  - `noteAccountBlockedLocked` caller — same hard-kinds condition → rate-limited account never enters the durable block ledger.

## 7. Fix Strategy

- `F-1` `finishTurn`: route `ProviderLimitRateLimited` into `quotaGateRunID`/`quotaGateTrigger` alongside the hard kinds. By construction, any kind reaching this code path already failed `isRecoverableSendError` — it is terminal-for-now regardless of kind.
- `F-2` `resolveQuotaGate`: carry `rate_limited` trigger into `demand.ObservedLimit` so the just-failed account hard-rejects in the same resolution pass.
- `F-3` `noteAccountBlockedLocked`: extend the live-observed block to `rate_limited` (semantics = quota_exhausted-style "unusable until fresh telemetry/headroom proves the window refilled" — not billing-permanent).
- `F-4` (optional, follow-up) persist `RetryAfterSeconds` in the block ledger so rotation can schedule an un-block at reset time.

## 7a. Code Guide Signatures

```go
// internal/runner/interactive_service.go — finishTurn (hard-kinds switch)
// add ProviderLimitRateLimited to quotaGateTrigger cases — F-1

// internal/runner/quota_gate.go — resolveQuotaGate (ObservedLimit switch)
// add ProviderLimitRateLimited to demand.ObservedLimit mapping — F-2

// internal/runner/interactive_service.go — noteAccountBlockedLocked call site
// extend kind condition to include ProviderLimitRateLimited — F-3
```

## 8. Validation

- `V-1` Repro: fake adapter returns `providerLimitError{Kind: rate_limited, RetryAfterSeconds: 2220}` → turn_failed → `enterQuotaGate` invoked (gate resolution recorded).
- `V-2` Task-445 invariant: `rate_limited` with `RetryAfterSeconds ∈ (0,30]` still retried in-turn; gate only fires when retries exhaust or are inapplicable.
- `V-3` Near-miss: transient `rate_limited` surviving all send attempts also enters the gate.
- `V-4` After fix, run-616108-shaped run resumes cleanly post-reset (manual verification on next live window).

## 8a. Test Signatures

- `TestBug661_LongRateLimitEntersQuotaGate` — rate_limited/RetryAfter=2220 → turn fails → quota gate resolved + run not terminal-failed (assert `enterQuotaGate` path reached via resolution record / gate outcome).
- `TestBug661_TransientRateLimitStillInTurnRetried` — rate_limited/RetryAfter=5 → `isRecoverableSendError` true → retried in-turn; gate not entered on success.
- `TestBug661_ExhaustedTransientRateLimitEntersGate` — rate_limited/RetryAfter=5 but SendTurn keeps failing through all attempts → gate entered.
- `TestBug661_RateLimitedAccountBlocked` — after the failing turn, `loadQuotaRuntimeState().accountBlockReason(acct)` non-empty for the pinned account.

## 9. Regression Guard

- tests: `bug661_*_test.go` + existing Task-445/447/449 clusters (`provider_limit`, `quota_gate`, `quota_claim` tests) must stay green.
- alerts: `[quota-gate] post-failure gate` log line should now also fire for `trigger=rate_limited`.
- audit checks: a run must never go `failed` from a provider limit without a corresponding `quota_gate` resolution or gate-record entry.

## 10. Definition of Done

- [ ] Root cause confirmed with evidence (§6) — confirmed: switch exclusion + live run-616108 event seq.
- [ ] §7a signatures landed; §8a tests exist, green, additive-only.
- [ ] Every pre-existing test untouched and green.
- [ ] Provider parity: provider_limit classifier is provider-agnostic (structured_payload + error-string fallback already shared).
- [ ] CA ledger entry written under `runner` feature_key.

## 11. Follow-Up Document Updates

- `change-audit/CA-*.md` — new entry on fix.
- Consider documenting the two-tier policy (in-turn ≤30s retry → post-failure quota gate) in `provider_limit.go` header so the exclusion can't silently drift again.
