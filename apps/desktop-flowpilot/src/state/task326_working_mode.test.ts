import test from "node:test";
import assert from "node:assert/strict";
import {
  DEV_HARNESS_FIVE,
  flowPickerOptions,
  loadWorkingMode,
  persistWorkingMode,
  wireWorkingMode,
} from "./workingMode";

test("chrome toggle vibe filters picker to vibe entries", () => {
  const ids = flowPickerOptions("vibe");
  // CP-90: vibe-tasks joins the two ingest entries; Task-459: vibe-adopt
  // is the fourth entry (adopt code written outside FlowPilot).
  assert.deepEqual(ids, ["vibe-ingest", "vibe-cp-ingest", "vibe-tasks", "vibe-adopt"]);
});

test("chrome toggle normal hides vibe-*", () => {
  const ids = flowPickerOptions("dev");
  assert.equal(ids.length, 5);
  assert.deepEqual(ids, [...DEV_HARNESS_FIVE]);
  assert.equal(ids.some((id) => id.startsWith("vibe-")), false);
});

// Clone-keeps-role: the armed-bar picker appends vibe-rooted clone refs
// (catalog UUIDs) after the static entries; no-arg behavior is unchanged.
test("flow picker appends vibe-rooted clone refs when provided", () => {
  assert.deepEqual(flowPickerOptions("vibe", ["clone-uuid-1"]), [
    "vibe-ingest",
    "vibe-cp-ingest",
    "vibe-tasks",
    "vibe-adopt",
    "clone-uuid-1",
  ]);
  assert.deepEqual(flowPickerOptions("dev", ["clone-uuid-1"]), [...DEV_HARNESS_FIVE, "clone-uuid-1"]);
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
