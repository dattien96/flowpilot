import test from "node:test";
import assert from "node:assert/strict";
import { stepRuntimeNeedsPoll } from "./flowStepRuntimePoll";

// BUG-1198: the fallback poll must stay alive while any step can still drift
// (RUNNING, PENDING, WAITING_USER_APPROVAL — the stale-card case) and stop on
// an all-terminal list so a settled run never burns requests.
test("stepRuntimeNeedsPoll: polls while any step is non-terminal", () => {
  assert.equal(stepRuntimeNeedsPoll([{ status: "RUNNING" }], true), true);
  assert.equal(stepRuntimeNeedsPoll([{ status: "DONE" }, { status: "PENDING" }], true), true);
  // WAITING_USER_APPROVAL parks can resolve elsewhere (card resolved on
  // another device / gate decision POST) — the rail must keep catching up.
  assert.equal(stepRuntimeNeedsPoll([{ status: "WAITING_USER_APPROVAL" }], true), true);
});

test("stepRuntimeNeedsPoll: stops once every step is terminal", () => {
  assert.equal(
    stepRuntimeNeedsPoll([{ status: "DONE" }, { status: "SKIPPED" }, { status: "FAILED" }, { status: "CANCELED" }], true),
    false,
  );
});

test("stepRuntimeNeedsPoll: empty list on a live run still polls (mount race), no run never polls", () => {
  assert.equal(stepRuntimeNeedsPoll([], true), true);
  assert.equal(stepRuntimeNeedsPoll([], false), false);
  assert.equal(stepRuntimeNeedsPoll([{ status: "RUNNING" }], false), false);
});
