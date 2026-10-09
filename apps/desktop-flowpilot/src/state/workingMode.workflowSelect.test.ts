// Workflow dropdown hygiene: engine-mounted vibe system flows (vibe-sprint,
// vibe-adopt-sprint, vibe-owner-debate) exist as catalog mirror rows and a
// reopened run's durable flowRef resolves to them. They are never a user
// pick — the select's option list must not grow an internal row just because
// a run happens to be driven by one.
import test from "node:test";
import assert from "node:assert/strict";
import { filterWorkflowsForWorkingMode, isSystemVibeFlowId, userFlowSelectableForMode, vibeCloneIds, workflowSelectOptions } from "./workingMode";

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

// A clone keeps its source's role: rows whose cloned_from chain roots at a
// vibe user-startable built-in count as vibe-startable in vibe mode.
const CLONE_ROWS = [
  { id: "mirror-tasks-uuid", packFlowId: "vibe-tasks" },
  { id: "clone-uuid-1", clonedFrom: "mirror-tasks-uuid" },
  { id: "clone-uuid-2", clonedFrom: "clone-uuid-1" },
  { id: "mirror-sprint-uuid", packFlowId: "vibe-sprint" },
  { id: "clone-uuid-sprint", clonedFrom: "mirror-sprint-uuid" },
  { id: "plain-uuid" },
  { id: "orphan-clone", clonedFrom: "gone-uuid" },
];

test("vibeCloneIds resolves clone ancestry to vibe roots", () => {
  const ids = vibeCloneIds(CLONE_ROWS);
  assert.ok(ids.has("clone-uuid-1"));
  assert.ok(ids.has("clone-uuid-2"));
  // A sprint clone keeps the system role: admittable as a user-owned copy.
  assert.ok(ids.has("clone-uuid-sprint"));
  assert.ok(!ids.has("plain-uuid"));
  assert.ok(!ids.has("orphan-clone"));
  assert.ok(!ids.has("mirror-tasks-uuid"));
});

test("vibe flow list includes clones of vibe flows", () => {
  const rows = [...CLONE_ROWS, { id: "wf-ingest", packFlowId: "vibe-ingest" }];
  const ids = filterWorkflowsForWorkingMode(rows, "vibe", vibeCloneIds(rows)).map((w) => w.id).sort();
  assert.deepEqual(ids, ["clone-uuid-1", "clone-uuid-2", "clone-uuid-sprint", "mirror-tasks-uuid", "wf-ingest"]);
});

test("userFlowSelectableForMode admits vibe-rooted clones in vibe mode", () => {
  const ids = vibeCloneIds(CLONE_ROWS);
  assert.ok(userFlowSelectableForMode("vibe", "clone-uuid-1", ids));
  assert.ok(userFlowSelectableForMode("vibe", "clone-uuid-sprint", ids));
  assert.ok(!userFlowSelectableForMode("vibe", "plain-uuid", ids));
  assert.ok(!userFlowSelectableForMode("vibe", "clone-uuid-1"));
  assert.ok(userFlowSelectableForMode("dev", "clone-uuid-1", ids));
});
