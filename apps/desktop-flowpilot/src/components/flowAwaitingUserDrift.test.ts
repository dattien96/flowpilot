import assert from "node:assert/strict";
import test from "node:test";
import { awaitingUserDriftState, parseDriftedPaths } from "./flowAwaitingUserDrift";

test("parseDriftedPaths: nil when not a drift gate", () => {
  assert.equal(parseDriftedPaths("Reviewer requested escalate"), null);
});

test("parseDriftedPaths: single and multi path", () => {
  assert.deepEqual(
    parseDriftedPaths("flow scope drift: wrote outside the frozen contract's declared paths: calc_test.go"),
    ["calc_test.go"],
  );
  assert.deepEqual(
    parseDriftedPaths("flow scope drift: wrote outside the frozen contract's declared paths: a.go, b.go"),
    ["a.go", "b.go"],
  );
});

test("parseDriftedPaths: trims trailing period and empty segments", () => {
  assert.deepEqual(
    parseDriftedPaths("wrote outside the frozen contract's declared paths: calc_test.go."),
    ["calc_test.go"],
  );
  assert.equal(parseDriftedPaths("wrote outside the frozen contract's declared paths: "), null);
});

test("awaitingUserDriftState: Allow only on non-cap non-stalled drift", () => {
  const drift = awaitingUserDriftState({
    blockReason: "escalate",
    gateReason: "flow scope drift: wrote outside the frozen contract's declared paths: calc_test.go",
  });
  assert.equal(drift.isDrift, true);
  assert.deepEqual(drift.driftedPaths, ["calc_test.go"]);
  assert.equal(drift.retryIsPrimary, false);

  const cap = awaitingUserDriftState({ blockReason: "cap", gateReason: "" });
  assert.equal(cap.isDrift, false);
  assert.equal(cap.retryIsPrimary, true);

  const stalled = awaitingUserDriftState({
    blockReason: "member_stalled",
    gateReason: "flow scope drift: wrote outside the frozen contract's declared paths: a.go",
  });
  assert.equal(stalled.isDrift, false);
  assert.equal(stalled.stalled, true);

  const escalate = awaitingUserDriftState({
    blockReason: "escalate",
    gateReason: "Reviewer requested escalate: missing clamp",
  });
  assert.equal(escalate.isDrift, false);
  assert.equal(escalate.retryIsPrimary, true);
});

test("awaitingUserDriftState: vibe_sprint_boundary flagged so the card can label Continue", () => {
  // CA-1097 (live run-3362): a finished vibe sprint parks on
  // vibe_sprint_boundary; the action is "start the next task", not a retry —
  // the desktop button read "Retry" while the TUI already renders [Continue].
  const boundary = awaitingUserDriftState({ blockReason: "vibe_sprint_boundary" });
  assert.equal(boundary.isSprintBoundary, true);
  assert.equal(boundary.stalled, false);
  assert.equal(boundary.isCap, false);
});
