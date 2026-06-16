import test from "node:test";
import assert from "node:assert/strict";

import { resolveQuestionManualSubmit } from "../../apps/desktop-flowpilot/src/components/questionAnswer";

test("single-select manual submit uses typed other text instead of a previous option", () => {
  assert.equal(resolveQuestionManualSubmit(["Python"], " Rust ", false), "Rust");
});

test("single-select manual submit waits for typed other text", () => {
  assert.equal(resolveQuestionManualSubmit(["Python"], "", false), undefined);
});

test("multi-select manual submit preserves selected options and appends typed other text", () => {
  assert.deepEqual(resolveQuestionManualSubmit(["Python"], "Rust", true), ["Python", "Rust"]);
});
