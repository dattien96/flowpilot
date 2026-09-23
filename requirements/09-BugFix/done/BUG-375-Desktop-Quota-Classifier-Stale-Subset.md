# BUG-375: Desktop quota classifier is a stale subset — Grok/OpenCode quota never opens account-switch

## Metadata

- Document ID: `BUG-375`
- Title: `Desktop isUsageLimitMessage misses payment/limit tokens — account-switch surface never fires for Grok/OpenCode quota`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-23`
- Last Updated: `2026-09-23`
- Feature Keys: `ai-providers`
- Parent Documents: CP-84 (surfaced during live validation, paired with BUG-374)
- Child Documents: `none`
- Related Documents: BUG-374 (runner-side detail drop), BUG-361 (OpenCode quota shapes)
- Replaces: `none`
- Tags: `desktop, quota, account-switch, classifier, provider-parity`

## AI Quick View

### Summary

- Desktop `isUsageLimitMessage` (store.ts) accepted only 6 tokens; the runner's `isProviderUsageLimitError` accepts ~20.
- Missing: `payment required`, `payment_required`, `no payment method`, `add a payment method`, `rate_limit`, `rate-limit`, `rate_limited`, `quota exceeded`, `quota_exceeded`, `insufficient credit`, `insufficient_credit`, `usage_limit`, `usage-limit`, `personal-team-blocked`, `spending-limit`, `spending_limit`.
- Consequence: even when the runner surfaces a correct quota message (post-BUG-374: `"Internal error: API error (status 402 Payment Required): …"`), the desktop `turn_failed` branch at `consumeStream` never opens `pendingAccountSwitch` (focused) nor ingests a quota inbox item (non-focused). The user sees a bare failure — consistent with the CP-81 "suddenly kicked out / silent provider death" symptom class.

## 4. Expected vs Actual

- expected: any runner-classified quota error opens the account-switch flow on the desktop.
- actual: only Claude-style "usage limit reached"/"rate limit" texts matched; Grok 402 and OpenCode billing copy fell through to a generic failed run.

## 5. Root Cause

Two classifiers gating the same flow drifted: the runner's grew (BUG-361, CP-46 Grok tokens) while the desktop's stayed at the original six.

## 6. Fix

`src/state/store.ts` `isUsageLimitMessage` — exported and expanded to the full runner token set, with a comment pinning parity to `isProviderUsageLimitError`.

## 7. Verification

- New `src/state/usageLimitClassifier.test.ts` (2 tests): full token set incl. the live Grok 402 and OpenCode billing copies match; healthy errors (`network timeout`, `exit status 1`, `Internal error: boom`, `grok turn ended: error`) do not.
- Provider-agnostic: the classifier is a pure string predicate; all provider error paths funnel through the same `turn_failed` branch.

## 8. Provider Parity Evidence

- Desktop `turn_failed` handling is provider-agnostic (no providerKey branch at the call site) — one classifier covers Claude/Codex/Grok/OpenCode/Gemini/Devin.
- Token set diffed 1:1 against `isProviderUsageLimitError` (22 tokens both sides).
