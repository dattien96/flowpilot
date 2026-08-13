import { describe, expect, it } from "vitest";

import { grokQuotaFromBilling } from "./_account-metadata";

describe("grokQuotaFromBilling credits format", () => {
  it("treats omitted creditUsagePercent as 100% remaining weekly limit", () => {
    expect(
      grokQuotaFromBilling({
        config: {
          currentPeriod: {
            type: "USAGE_PERIOD_TYPE_WEEKLY",
            end: "2026-08-19T23:46:37.792216+00:00",
          },
          billingPeriodEnd: "2026-08-19T23:46:37.792216+00:00",
        },
      }),
    ).toMatchObject({
      label: "Weekly limit",
      remainingPercent: 100,
    });
  });

  it("maps creditUsagePercent used-percent to remaining", () => {
    expect(
      grokQuotaFromBilling({
        config: {
          creditUsagePercent: 1,
          currentPeriod: {
            type: "USAGE_PERIOD_TYPE_WEEKLY",
            end: "2026-07-16T07:31:03.807421+00:00",
          },
        },
      }),
    ).toMatchObject({
      label: "Weekly limit",
      remainingPercent: 99,
    });
  });
});
