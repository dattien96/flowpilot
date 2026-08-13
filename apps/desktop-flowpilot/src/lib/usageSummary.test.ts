import test from "node:test";
import assert from "node:assert/strict";
import { contextRemainingPercent, formatAccountRemainingLabel } from "./usageSummary";

test("contextRemainingPercent matches Grok-style remaining window percent", () => {
  assert.equal(contextRemainingPercent(12000, 128000), 91);
  assert.equal(contextRemainingPercent(128000, 128000), 0);
  assert.equal(contextRemainingPercent(0, 100000), 100);
  assert.equal(contextRemainingPercent(10, 0), 0);
});

test("formatAccountRemainingLabel prefers weekly credits like Grok /usage", () => {
  assert.equal(
    formatAccountRemainingLabel({ remaining7dPercent: 87, remaining5hPercent: 40 }),
    "7d 87%",
  );
  assert.equal(
    formatAccountRemainingLabel({ remaining7dPercent: null, remaining5hPercent: 40 }),
    "5h 40%",
  );
  assert.equal(
    formatAccountRemainingLabel({
      remaining7dPercent: null,
      remaining5hPercent: null,
      usageDetailLines: [{ label: "Weekly limit", remainingPercent: 99 }],
    }),
    "Weekly limit 99%",
  );
  assert.equal(formatAccountRemainingLabel(null), null);
});
