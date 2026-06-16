import test from "node:test";
import assert from "node:assert/strict";
import {
  buildToastGroupSummary,
  COLLAPSIBLE_TOAST_THRESHOLD,
  shouldCollapseToasts,
} from "../../apps/desktop-flowpilot/src/app/runToastGrouping";

test("collapses once three or more notifications are active", () => {
  assert.equal(shouldCollapseToasts(COLLAPSIBLE_TOAST_THRESHOLD - 1), false);
  assert.equal(shouldCollapseToasts(COLLAPSIBLE_TOAST_THRESHOLD), true);
  assert.equal(shouldCollapseToasts(COLLAPSIBLE_TOAST_THRESHOLD + 2), true);
});

test("builds a stable grouped summary by notification kind", () => {
  assert.equal(
    buildToastGroupSummary([
      { kind: "approval" },
      { kind: "question" },
      { kind: "approval" },
      { kind: "done" },
    ]),
    "Completed 1 · Approvals 2 · Questions 1",
  );
});
