import assert from "node:assert/strict";
import test from "node:test";
import { isUsageLimitMessage } from "./store";

// BUG-375: the desktop quota classifier was a stale subset of the runner's
// isProviderUsageLimitError — Grok/OpenCode quota signatures ("payment
// required", "rate_limited", "quota_exceeded", "spending-limit", …) never
// matched, so the account-switch modal/inbox decision never fired even when
// the runner surfaced the right text. The token set must mirror the runner's
// (interactive_service.go isProviderUsageLimitError).

const MUST_MATCH = [
  "usage limit reached",
  "extra usage unavailable",
  "out of credits",
  "out_of_credits",
  "quota reset",
  "rate limit",
  "rate_limit",
  "rate-limit",
  "rate_limited",
  "quota exceeded",
  "quota_exceeded",
  "insufficient credit",
  "insufficient_credit",
  "usage_limit",
  "usage-limit",
  "payment required",
  "payment_required",
  "no payment method",
  "add a payment method",
  "personal-team-blocked",
  "spending-limit",
  "spending_limit",
  // Live Grok 402 (BUG-374 wire shape, detail now surfaced):
  "Internal error: API error (status 402 Payment Required): Grok Build usage balance exhausted",
  // Live OpenCode billing copy (BUG-361):
  "Internal error: No payment method. Add a payment method here: https://opencode.ai/workspace/x/billing",
];

const MUST_NOT_MATCH = [
  "network timeout",
  "exit status 1",
  "context canceled",
  "Internal error: boom",
  "grok turn ended: error",
  "",
];

test("usage-limit classifier matches runner token set", () => {
  for (const msg of MUST_MATCH) {
    assert.equal(isUsageLimitMessage(msg), true, `expected quota match: ${msg}`);
  }
});

test("usage-limit classifier rejects healthy errors", () => {
  for (const msg of MUST_NOT_MATCH) {
    assert.equal(isUsageLimitMessage(msg), false, `unexpected quota match: ${msg}`);
  }
});
