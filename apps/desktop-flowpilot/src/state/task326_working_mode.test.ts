import test from "node:test";
import assert from "node:assert/strict";
import {
  DEV_HARNESS_FIVE,
  flowPickerOptions,
  loadWorkingMode,
  persistWorkingMode,
  wireWorkingMode,
} from "./workingMode";

test("chrome toggle vibe filters picker to ingest", () => {
  const ids = flowPickerOptions("vibe");
  assert.deepEqual(ids, ["vibe-ingest"]);
});

test("chrome toggle normal hides vibe-*", () => {
  const ids = flowPickerOptions("dev");
  assert.equal(ids.length, 5);
  assert.deepEqual(ids, [...DEV_HARNESS_FIVE]);
  assert.equal(ids.some((id) => id.startsWith("vibe-")), false);
});

test("label Normal never sent on the wire", () => {
  assert.equal(wireWorkingMode("Normal"), "dev");
  assert.equal(wireWorkingMode("normal"), "dev");
  assert.notEqual(wireWorkingMode("Normal"), "normal");
});

test("chrome toggle persists next-start default", () => {
  persistWorkingMode("vibe");
  assert.equal(loadWorkingMode(), "vibe");
  persistWorkingMode("dev");
  assert.equal(loadWorkingMode(), "dev");
});
