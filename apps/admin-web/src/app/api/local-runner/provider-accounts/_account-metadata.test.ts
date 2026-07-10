import { describe, expect, it } from "vitest";

import {
  buildClaudeUsageSummary,
  grokQuotaFromBilling,
  mapGeminiQuotaUsageDetailLines,
} from "./_account-metadata";

describe("mapGeminiQuotaUsageDetailLines", () => {
  it("keeps gemini 3 quota buckets when more than three models are returned", () => {
    const lines = mapGeminiQuotaUsageDetailLines([
      {
        modelId: "gemini-3.1-pro-high",
        remainingFraction: 0.4,
        resetTime: "2026-06-03T00:03:00.000Z",
      },
      {
        modelId: "gemini-3.5-flash-medium",
        remainingFraction: 1,
        resetTime: "2026-06-03T00:03:00.000Z",
      },
      {
        modelId: "gemini-3.5-flash-low",
        remainingFraction: 1,
        resetTime: "2026-06-03T00:03:00.000Z",
      },
      {
        modelId: "gemini-3.1-pro-low",
        remainingFraction: 1,
        resetTime: "2026-06-03T00:03:00.000Z",
      },
      {
        modelId: "gemini-3.5-flash-high",
        remainingFraction: 0.6,
        resetTime: "2026-06-03T00:03:00.000Z",
      },
    ]);

    expect(lines.map((line) => line.label)).toEqual([
      "gemini-3.1-pro-high",
      "gemini-3.1-pro-low",
      "gemini-3.5-flash-high",
      "gemini-3.5-flash-low",
      "gemini-3.5-flash-medium",
    ]);
  });

  it("filters invalid buckets and normalizes percentages", () => {
    const lines = mapGeminiQuotaUsageDetailLines([
      {
        modelId: "gemini-3.1-pro-high",
        remainingFraction: 1.2,
        resetTime: "1780444800",
      },
      {
        modelId: "gemini-3.5-flash-medium",
        remainingFraction: -0.5,
        resetTime: "1780444800",
      },
      { modelId: null, remainingFraction: 0.4, resetTime: null },
      {
        modelId: "gemini-3.1-pro-low",
        remainingFraction: null,
        resetTime: null,
      },
    ]);

    expect(lines).toEqual([
      {
        label: "gemini-3.1-pro-high",
        remainingPercent: 100,
        resetAt: "2026-06-03T00:00:00.000Z",
      },
      {
        label: "gemini-3.5-flash-medium",
        remainingPercent: 0,
        resetAt: "2026-06-03T00:00:00.000Z",
      },
    ]);
  });
});

describe("buildClaudeUsageSummary", () => {
  it("prefers claude auth status subscription type and surfaces extra usage exhaustion", () => {
    expect(
      buildClaudeUsageSummary({
        account: {
          billingType: "stripe_subscription",
          hasExtraUsageEnabled: true,
          subscriptionCreatedAt: "2026-03-31T01:14:27.180375Z",
        },
        authStatus: {
          loggedIn: true,
          subscriptionType: "team",
        },
        extraUsageDisabledReason: "out_of_credits",
      }),
    ).toBe("Team since 2026-03-31 | extra usage unavailable (out of credits)");
  });

  it("falls back to current claude auth file fields when auth status is unavailable", () => {
    expect(
      buildClaudeUsageSummary({
        account: {
          billingType: "stripe_subscription",
          hasExtraUsageEnabled: true,
        },
        authStatus: null,
        extraUsageDisabledReason: null,
      }),
    ).toBe("Stripe Subscription | extra usage enabled");
  });
});

// Task-216: corrects CP-46 Q-5/Task-210 Q-1's "no machine-readable quota
// endpoint" finding — this is the exact response shape captured live from
// GET {cli-chat-proxy.grok.com}/v1/billing.
describe("grokQuotaFromBilling", () => {
  it("maps a monthly billing config to a remaining-percent line", () => {
    expect(
      grokQuotaFromBilling({
        config: {
          monthlyLimit: { val: 15000 },
          used: { val: 56 },
          billingPeriodEnd: "2026-08-01T00:00:00+00:00",
        },
      }),
    ).toEqual({
      label: "Team Credits (Monthly)",
      remainingPercent: 99,
      resetAt: "2026-08-01T00:00:00.000Z",
    });
  });

  it("falls back to weeklyLimit when monthlyLimit is absent", () => {
    expect(
      grokQuotaFromBilling({
        config: {
          weeklyLimit: { val: 100 },
          used: { val: 40 },
        },
      }),
    ).toMatchObject({
      label: "Team Credits (Weekly)",
      remainingPercent: 60,
    });
  });

  it("returns null when neither limit field is present", () => {
    expect(grokQuotaFromBilling({ config: {} })).toBeNull();
    expect(grokQuotaFromBilling(null)).toBeNull();
  });
});
