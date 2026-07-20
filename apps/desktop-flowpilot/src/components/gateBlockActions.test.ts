import assert from "node:assert/strict";
import test from "node:test";
import { gateBlockSecondaryAction } from "./gateBlockActions";

test("BUG-291: regression decision cards cannot orphan a blocked child with a local dismiss", () => {
  assert.equal(gateBlockSecondaryAction(["keep-test-fix-code", "suggest-requirement-change"]), "stop-flow");
});

test("BUG-291: plain informational gate blocks remain locally acknowledgeable", () => {
  assert.equal(gateBlockSecondaryAction(undefined), "dismiss");
  assert.equal(gateBlockSecondaryAction([]), "dismiss");
});
