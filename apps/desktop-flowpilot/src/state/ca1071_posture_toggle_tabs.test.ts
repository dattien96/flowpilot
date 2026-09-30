import test from "node:test";
import assert from "node:assert/strict";
import {
  CHAT_POSTURE_TABS,
  CHAT_POSTURES,
  postureAfterTabClick,
} from "../types/contract";

// CA-1071: the posture strip shows only Scan/Plan/Code as toggleable options —
// "non" is the unselected state, reached by clicking the active tab again.
test("posture tabs expose only scan/plan/code — non is the deselected state", () => {
  assert.deepEqual(CHAT_POSTURE_TABS.map((p) => p.key), ["scan", "plan", "code"]);
  // Hints for all four postures (incl. non) still resolve.
  assert.equal(CHAT_POSTURES.length, 4);
});

test("clicking the active posture tab deselects it to non", () => {
  assert.equal(postureAfterTabClick("plan", "plan"), "non");
  assert.equal(postureAfterTabClick("code", "code"), "non");
  assert.equal(postureAfterTabClick("scan", "scan"), "non");
});

test("clicking a different tab selects it; from non any tab selects", () => {
  assert.equal(postureAfterTabClick("plan", "code"), "code");
  assert.equal(postureAfterTabClick("non", "plan"), "plan");
  assert.equal(postureAfterTabClick("non", "scan"), "scan");
});
