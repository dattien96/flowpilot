import { describe, expect, it } from "vitest";

import { mapGeminiQuotaUsageDetailLines } from "./_account-metadata";

describe("mapGeminiQuotaUsageDetailLines", () => {
  it("keeps gemini 3 quota buckets when more than three models are returned", () => {
    const lines = mapGeminiQuotaUsageDetailLines([
      {
        modelId: "gemini-3.1-pro-preview",
        remainingFraction: 0.4,
        resetTime: "2026-06-03T00:03:00.000Z",
      },
      {
        modelId: "gemini-2.5-flash",
        remainingFraction: 1,
        resetTime: "2026-06-03T00:03:00.000Z",
      },
      {
        modelId: "gemini-2.5-flash-lite",
        remainingFraction: 1,
        resetTime: "2026-06-03T00:03:00.000Z",
      },
      {
        modelId: "gemini-2.5-pro",
        remainingFraction: 1,
        resetTime: "2026-06-03T00:03:00.000Z",
      },
      {
        modelId: "gemini-3-flash-preview",
        remainingFraction: 0.6,
        resetTime: "2026-06-03T00:03:00.000Z",
      },
    ]);

    expect(lines.map((line) => line.label)).toEqual([
      "gemini-2.5-flash",
      "gemini-2.5-flash-lite",
      "gemini-2.5-pro",
      "gemini-3-flash-preview",
      "gemini-3.1-pro-preview",
    ]);
  });

  it("filters invalid buckets and normalizes percentages", () => {
    const lines = mapGeminiQuotaUsageDetailLines([
      { modelId: "gemini-2.5-pro", remainingFraction: 1.2, resetTime: 1_780_444_800 },
      { modelId: "gemini-3-flash-preview", remainingFraction: -0.5, resetTime: "1780444800" },
      { modelId: null, remainingFraction: 0.4, resetTime: null },
      { modelId: "gemini-3.1-pro-preview", remainingFraction: null, resetTime: null },
    ]);

    expect(lines).toEqual([
      {
        label: "gemini-2.5-pro",
        remainingPercent: 100,
        resetAt: "2026-06-03T00:00:00.000Z",
      },
      {
        label: "gemini-3-flash-preview",
        remainingPercent: 0,
        resetAt: "2026-06-03T00:00:00.000Z",
      },
    ]);
  });
});
