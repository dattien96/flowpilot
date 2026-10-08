// Workflow dropdown hygiene: engine-mounted vibe system flows (vibe-sprint,
// vibe-adopt-sprint, vibe-owner-debate) exist as catalog mirror rows and a
// reopened run's durable flowRef resolves to them. They are never a user
// pick — the select's option list must not grow an internal row just because
// a run happens to be driven by one.
import test from "node:test";
import assert from "node:assert/strict";
import { isSystemVibeFlowId, workflowSelectOptions } from "./workingMode";

const USER_ROWS = [
  { id: "wf-ingest", packFlowId: "vibe-ingest" },
  { id: "wf-tasks", packFlowId: "vibe-tasks" },
  { id: "wf-adopt", packFlowId: "vibe-adopt" },
];
const SPRINT_ROW = { id: "wf-sprint", packFlowId: "vibe-sprint" };
const ADOPT_SPRINT_ROW = { id: "wf-adopt-sprint", packFlowId: "vibe-adopt-sprint" };
const CUSTOM_ROW = { id: "wf-custom-uuid" };

test("system vibe flow ids are detected bare or pack-prefixed", () => {
  assert.ok(isSystemVibeFlowId("vibe-sprint"));
  assert.ok(isSystemVibeFlowId("flowpilot-core-flow-pack/vibe-adopt-sprint"));
  assert.ok(isSystemVibeFlowId("vibe-owner-debate"));
  assert.ok(!isSystemVibeFlowId("vibe-adopt"));
  assert.ok(!isSystemVibeFlowId("vibe-tasks"));
  assert.ok(!isSystemVibeFlowId(undefined));
  assert.ok(!isSystemVibeFlowId("wf-custom-uuid"));
});

test("selected system-flow row is never appended to the option list", () => {
  const all = [...USER_ROWS, SPRINT_ROW, ADOPT_SPRINT_ROW];
  const opts = workflowSelectOptions(USER_ROWS, all, "wf-adopt-sprint");
  assert.deepEqual(opts.map((w) => w.id), USER_ROWS.map((w) => w.id));
});

test("selected non-startable NON-system row still appends (BUG-1199)", () => {
  const all = [...USER_ROWS, CUSTOM_ROW];
  const opts = workflowSelectOptions(USER_ROWS, all, "wf-custom-uuid");
  assert.deepEqual(opts.map((w) => w.id), [...USER_ROWS.map((w) => w.id), "wf-custom-uuid"]);
});

test("no selection or already-visible selection leaves options untouched", () => {
  assert.deepEqual(workflowSelectOptions(USER_ROWS, [...USER_ROWS, SPRINT_ROW], undefined), USER_ROWS);
  assert.deepEqual(workflowSelectOptions(USER_ROWS, USER_ROWS, "wf-tasks"), USER_ROWS);
});
