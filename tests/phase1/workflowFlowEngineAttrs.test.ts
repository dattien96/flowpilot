import test from "node:test";
import assert from "node:assert/strict";

import { SupabaseAdminRepository } from "../../packages/flowpilot-client-core/src/data/supabaseAdminRepository";

// Minimal fluent fake for supabase-js's query builder, scoped to what
// saveWorkflow/cloneWorkflow actually call. Each table has its own canned
// response per terminal method (maybeSingle/single/then-as-promise via
// insert/delete), configured per test.
class FakeTable {
  public lastEqFilters: Record<string, unknown> = {};
  public lastPayload: unknown;
  public insertPayloads: unknown[] = [];
  public updatePayloads: unknown[] = [];

  constructor(
    private readonly responses: {
      maybeSingle?: { data?: any; error?: any };
      single?: { data?: any; error?: any };
      list?: { data?: any; error?: any };
      write?: { data?: any; error?: any };
    },
  ) {}

  select() {
    return this;
  }
  eq(column: string, value: unknown) {
    this.lastEqFilters[column] = value;
    return this;
  }
  in(column: string, values: unknown[]) {
    this.lastEqFilters[column] = values;
    return this;
  }
  not() {
    return this;
  }
  order() {
    return Promise.resolve(this.responses.list ?? { data: [], error: null });
  }
  maybeSingle() {
    return Promise.resolve(this.responses.maybeSingle ?? { data: null, error: null });
  }
  single() {
    return Promise.resolve(this.responses.single ?? { data: null, error: null });
  }
  upsert(payload: unknown) {
    this.lastPayload = payload;
    return this;
  }
  insert(payload: unknown) {
    this.lastPayload = payload;
    this.insertPayloads.push(payload);
    return this;
  }
  update(payload: unknown) {
    this.lastPayload = payload;
    this.updatePayloads.push(payload);
    return this;
  }
  delete() {
    return this;
  }
  // Makes the builder itself awaitable when no terminal method (single/
  // maybeSingle/order) is called — matches supabase-js's PostgrestBuilder,
  // which is thenable. Used by callers like `.delete().eq(...)` that never
  // call a terminal method and just `await` the filter chain.
  then(resolve: (value: any) => void, reject?: (reason: any) => void) {
    Promise.resolve(this.responses.write ?? { data: null, error: null }).then(resolve, reject);
  }
}

class FakeSupabase {
  public tables = new Map<string, FakeTable>();

  register(table: string, fake: FakeTable) {
    this.tables.set(table, fake);
    return fake;
  }

  from(table: string) {
    const fake = this.tables.get(table);
    if (!fake) throw new Error(`no fake registered for table ${table}`);
    return fake;
  }
}

function builtinWorkflowRow(overrides: Record<string, unknown> = {}) {
  return {
    id: "wf-builtin-1",
    project_id: null,
    name: "Review Loop",
    description: "review until clean",
    is_template: false,
    provider_override: null,
    model_override: null,
    reasoning_effort_override: null,
    yolo_mode: false,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    is_builtin: true,
    editable: false,
    cloneable: true,
    cloned_from: null,
    pack_id: "flowpilot-core-flow-pack",
    pack_version: "0.1.0",
    pack_flow_id: "review-loop",
    pack_hash: "abc123",
    selectable_in_json: ["chat", "flow"],
    chat_baseline: false,
    chat_sub_modes_json: ["bug"],
    policy_cap: 3,
    policy_on_cap: "escalate",
    policy_extend_by: 2,
    policy_extend_max: 2,
    ...overrides,
  };
}

test("saveWorkflow limits built-in workflow saves to override fields only", async () => {
  const supabase = new FakeSupabase();
  const workflowsTable = new FakeTable({
    maybeSingle: { data: { editable: false }, error: null },
    single: {
      data: builtinWorkflowRow({
        model_override: "gpt-5.4",
        reasoning_effort_override: "high",
        yolo_mode: true,
        name: "Review Loop",
      }),
      error: null,
    },
  });
  let updatePayload: Record<string, unknown> | null = null;
  (workflowsTable as any).update = function (payload: Record<string, unknown>) {
    updatePayload = payload;
    return this;
  };
  supabase.register(
    "workflows",
    workflowsTable,
  );
  const repo = new SupabaseAdminRepository(supabase as any);

  const saved = await repo.saveWorkflow({
    id: "wf-builtin-1",
    name: "hacked name",
    modelOverride: "gpt-5.4",
    reasoningEffortOverride: "high",
    yoloMode: true,
  });

  assert.deepEqual(updatePayload, {
    model_override: "gpt-5.4",
    reasoning_effort_override: "high",
    yolo_mode: true,
    updated_at: updatePayload?.updated_at,
  });
  assert.equal(saved.name, "Review Loop");
  assert.equal(saved.modelOverride, "gpt-5.4");
  assert.equal(saved.reasoningEffortOverride, "high");
  assert.equal(saved.yoloMode, true);
});

test("saveWorkflow allows editing when editable=true", async () => {
  const supabase = new FakeSupabase();
  supabase.register(
    "workflows",
    new FakeTable({
      maybeSingle: { data: { editable: true }, error: null },
      single: { data: builtinWorkflowRow({ id: "wf-user-1", is_builtin: false, editable: true, name: "My Flow" }), error: null },
    }),
  );
  const repo = new SupabaseAdminRepository(supabase as any);

  const saved = await repo.saveWorkflow({ id: "wf-user-1", name: "My Flow" });
  assert.equal(saved.name, "My Flow");
  assert.equal(saved.editable, true);
});

test("cloneWorkflow creates an editable, non-builtin copy referencing the source", async () => {
  const supabase = new FakeSupabase();
  const sourceRow = builtinWorkflowRow();
  const clonedRow = builtinWorkflowRow({
    id: "wf-clone-1",
    is_builtin: false,
    editable: true,
    cloned_from: "wf-builtin-1",
    name: "My Review Loop",
  });

  // cloneWorkflow calls .single() twice on "workflows": once to load the
  // source row, once for the insert's returned row. Respond in that order.
  const workflowsResponses = [sourceRow, clonedRow];
  const workflowsTable = new FakeTable({});
  (workflowsTable as any).single = () =>
    Promise.resolve({ data: workflowsResponses.shift(), error: null });
  supabase.register("workflows", workflowsTable);
  supabase.register("workflow_steps", new FakeTable({ list: { data: [], error: null } }));

  const repo = new SupabaseAdminRepository(supabase as any);
  const cloned = await repo.cloneWorkflow("wf-builtin-1", "My Review Loop");

  assert.equal(cloned.isBuiltin, false);
  assert.equal(cloned.editable, true);
  assert.equal(cloned.clonedFrom, "wf-builtin-1");
  assert.equal(cloned.name, "My Review Loop");
});

// Regression test for BUG-262 (+ BUG-282): cloneWorkflow used to reuse the
// source's step_type verbatim for the clone's workflow_steps, so both
// workflows pointed at the SAME step_definitions row; editing the clone's
// edges then silently overwrote the source's shared fields. The fix gives
// every cloned step its own fresh, workflow-scoped step_type and an
// independent step_definitions row. BUG-282 additionally removes flow
// topology from step_definitions entirely — `depends_on_json` is no longer a
// column and the clone must NOT copy it (topology lives only on the workflow's
// own edges_json, which cloneWorkflow copies onto the clone's row).
test("cloneWorkflow gives every cloned step its own step_definitions row, not the source's", async () => {
  const supabase = new FakeSupabase();
  const sourceRow = builtinWorkflowRow();
  const clonedRow = builtinWorkflowRow({
    id: "wf-clone-1",
    is_builtin: false,
    editable: true,
    cloned_from: "wf-builtin-1",
    name: "My Review Loop",
  });
  const workflowsResponses = [sourceRow, clonedRow];
  const workflowsTable = new FakeTable({});
  (workflowsTable as any).single = () =>
    Promise.resolve({ data: workflowsResponses.shift(), error: null });
  supabase.register("workflows", workflowsTable);

  const sourceStepType = "flowpilot_core_flow_pack_review_loop_synthesis";
  supabase.register(
    "workflow_steps",
    new FakeTable({
      list: {
        data: [
          {
            id: "wfstep-1",
            workflow_id: "wf-builtin-1",
            step_type: sourceStepType,
            order_index: 3,
            is_enabled: true,
            requires_approval: true,
          },
        ],
        error: null,
      },
    }),
  );
  const stepDefinitionsTable = new FakeTable({
    write: {
      data: [
        {
          step_type: sourceStepType,
          name: "Review Loop: Synthesis",
          description: "synthesis node",
          node_id: "synthesis",
          behavior_id: "hub.inline",
          agent_ref: "agents/synthesizer.md",
          node_lifecycle: "reinvoke",
          depends_on_json: ["reviewer_correctness", "reviewer_security"],
          model: "claude-haiku",
        },
      ],
      error: null,
    },
  });
  supabase.register("step_definitions", stepDefinitionsTable);

  const repo = new SupabaseAdminRepository(supabase as any);
  const cloned = await repo.cloneWorkflow("wf-builtin-1", "My Review Loop");

  const newDefinitionRows = stepDefinitionsTable.insertPayloads[0] as Array<Record<string, unknown>>;
  assert.equal(newDefinitionRows.length, 1);
  const newStepType = newDefinitionRows[0].step_type as string;
  assert.notEqual(newStepType, sourceStepType, "clone must not reuse the source's step_type");
  assert.equal(newStepType, `${cloned.id}__${sourceStepType}`);
  assert.equal(newDefinitionRows[0].node_id, "synthesis");
  // BUG-282: flow topology is not stored on the step definition, so the clone
  // must not copy depends_on_json — even though the seeded source row (a legacy
  // row from before the column was dropped) still carries it.
  assert.ok(
    !("depends_on_json" in newDefinitionRows[0]),
    "clone must not copy flow topology onto the cloned step definition",
  );

  const workflowStepsTable = supabase.tables.get("workflow_steps")!;
  const newRelationRows = workflowStepsTable.insertPayloads[0] as Array<Record<string, unknown>>;
  assert.equal(newRelationRows.length, 1);
  assert.equal(newRelationRows[0].step_type, newStepType);
  assert.equal(newRelationRows[0].workflow_id, cloned.id);
});

// Regression test for BUG-NOTE-CP42 #18: saveWorkflow used to DELETE every
// existing workflow_steps row before inserting the replacements, so an
// insert failure permanently lost the old steps (the delete had already
// committed). This asserts the call ordering directly: insert must happen
// before delete, and — per the fix — a failed insert must never be followed
// by a delete call at all.
test("saveWorkflow inserts new steps before deleting superseded ones", async () => {
  const supabase = new FakeSupabase();
  supabase.register(
    "workflows",
    new FakeTable({
      maybeSingle: { data: null, error: null },
      single: { data: builtinWorkflowRow({ id: "wf-1", is_builtin: false, editable: true }), error: null },
    }),
  );
  const callOrder: string[] = [];
  const stepsTable = new FakeTable({ write: { data: [{ id: "step-1" }], error: null } });
  (stepsTable as any).insert = function (payload: unknown) {
    callOrder.push("insert");
    this.lastPayload = payload;
    this.insertPayloads.push(payload);
    return this;
  };
  (stepsTable as any).delete = function () {
    callOrder.push("delete");
    return this;
  };
  (stepsTable as any).update = function (payload: unknown) {
    callOrder.push("update");
    this.lastPayload = payload;
    this.updatePayloads.push(payload);
    return this;
  };
  supabase.register("workflow_steps", stepsTable);
  const repo = new SupabaseAdminRepository(supabase as any);

  await repo.saveWorkflow({
    id: "wf-1",
    name: "wf",
    steps: [{ stepType: "coding" }],
  });

  assert.deepEqual(callOrder, ["insert", "delete", "update"]);
});

test("saveWorkflow offsets replacement step order_index before renormalizing", async () => {
  const supabase = new FakeSupabase();
  supabase.register(
    "workflows",
    new FakeTable({
      maybeSingle: { data: null, error: null },
      single: { data: builtinWorkflowRow({ id: "wf-1", is_builtin: false, editable: true }), error: null },
    }),
  );
  const stepsTable = new FakeTable({ write: { data: [{ id: "step-1" }, { id: "step-2" }], error: null } });
  supabase.register("workflow_steps", stepsTable);
  const repo = new SupabaseAdminRepository(supabase as any);

  await repo.saveWorkflow({
    id: "wf-1",
    name: "wf",
    steps: [{ stepType: "coding", orderIndex: 0 }, { stepType: "review", orderIndex: 1 }],
  });

  assert.deepEqual(stepsTable.lastPayload, { order_index: 1 });
  assert.deepEqual(stepsTable.updatePayloads, [{ order_index: 0 }, { order_index: 1 }]);
  const insertPayload = stepsTable.insertPayloads[0] as Array<Record<string, unknown>> | undefined;
  assert.equal(Array.isArray(insertPayload), true);
  assert.deepEqual(
    insertPayload?.map((row) => row.order_index),
    [1_000_000, 1_000_001],
  );
});

test("saveWorkflow does not delete existing steps when the insert fails", async () => {
  const supabase = new FakeSupabase();
  supabase.register(
    "workflows",
    new FakeTable({
      maybeSingle: { data: null, error: null },
      single: { data: builtinWorkflowRow({ id: "wf-1", is_builtin: false, editable: true }), error: null },
    }),
  );
  let sawDelete = false;
  const stepsTable = new FakeTable({ write: { data: null, error: new Error("simulated FK violation") } });
  (stepsTable as any).delete = function () {
    sawDelete = true;
    return this;
  };
  supabase.register("workflow_steps", stepsTable);
  const repo = new SupabaseAdminRepository(supabase as any);

  await assert.rejects(() =>
    repo.saveWorkflow({
      id: "wf-1",
      name: "wf",
      steps: [{ stepType: "coding" }],
    }),
  );

  assert.equal(sawDelete, false, "existing steps must not be deleted when the replacement insert failed");
});

// Task-189 (owner-confirmed 2026-07-06, = BUG-236 contract): node identity
// (nodeId/behaviorId/agentRef/joinMode/cohort) lives ONLY on StepDefinition —
// WorkflowStep/workflow_steps is a pure relation/order table and must never
// carry node data. (BUG-282: flow topology, formerly `depends_on_json`, no
// longer lives on StepDefinition either — it lives only on the workflow's
// edges_json — so workflow_steps still must never carry it.) This test used
// to be named
// "mapWorkflowStep round-trips the new flow-engine attrs" and asserted the
// OPPOSITE (that saveWorkflow wrote node_id/behavior_id/etc into the
// workflow_steps insert payload) — that was the design BUG-236 explicitly
// rejected. Rewritten to assert the corrected contract instead of the
// rejected one.
test("saveWorkflow never writes node-identity fields into workflow_steps (BUG-236)", async () => {
  const supabase = new FakeSupabase();
  supabase.register(
    "workflows",
    new FakeTable({
      maybeSingle: { data: null, error: null },
      single: { data: builtinWorkflowRow({ id: "wf-1", is_builtin: false, editable: true }), error: null },
    }),
  );
  const stepsTable = new FakeTable({ write: { data: null, error: null } });
  supabase.register("workflow_steps", stepsTable);
  const repo = new SupabaseAdminRepository(supabase as any);

  await repo.saveWorkflow({
    id: "wf-1",
    name: "wf",
    steps: [{ stepType: "coding", orderIndex: 0, isEnabled: true, requiresApproval: false }],
  });

  const payload = stepsTable.insertPayloads[0] as Array<Record<string, unknown>>;
  assert.deepEqual(Object.keys(payload[0]).sort(), [
    "is_enabled",
    "order_index",
    "requires_approval",
    "step_type",
    "workflow_id",
  ]);
  assert.equal(payload[0].step_type, "coding");
  assert.equal(payload[0].order_index, 1_000_000);
  assert.equal(payload[0].is_enabled, true);
  assert.equal(payload[0].requires_approval, false);
  for (const nodeField of ["node_id", "behavior_id", "agent_ref", "depends_on_json", "join_mode", "cohort"]) {
    assert.equal(nodeField in payload[0], false, `workflow_steps payload must never carry "${nodeField}"`);
  }
});
