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

test("saveWorkflow rejects editing a non-editable (built-in) workflow", async () => {
  const supabase = new FakeSupabase();
  supabase.register(
    "workflows",
    new FakeTable({ maybeSingle: { data: { editable: false }, error: null } }),
  );
  const repo = new SupabaseAdminRepository(supabase as any);

  await assert.rejects(
    () => repo.saveWorkflow({ id: "wf-builtin-1", name: "hacked name" }),
    /built-in template/,
  );
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
    return this;
  };
  (stepsTable as any).delete = function () {
    callOrder.push("delete");
    return this;
  };
  supabase.register("workflow_steps", stepsTable);
  const repo = new SupabaseAdminRepository(supabase as any);

  await repo.saveWorkflow({
    id: "wf-1",
    name: "wf",
    steps: [{ stepType: "coding", nodeId: "coder", behaviorId: "agent.delegate", agentRef: "agents/coder.md", dependsOn: [] }],
  });

  assert.deepEqual(callOrder, ["insert", "delete"]);
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
      steps: [{ stepType: "coding", nodeId: "coder", behaviorId: "agent.delegate", agentRef: "agents/coder.md", dependsOn: [] }],
    }),
  );

  assert.equal(sawDelete, false, "existing steps must not be deleted when the replacement insert failed");
});

test("mapWorkflowStep round-trips the new flow-engine attrs", async () => {
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
    steps: [
      {
        stepType: "coding",
        nodeId: "coder",
        behaviorId: "agent.delegate",
        agentRef: "agents/coder.md",
        dependsOn: [],
        joinMode: "all",
      },
    ],
  });

  const payload = stepsTable.lastPayload as Array<Record<string, unknown>>;
  assert.equal(payload[0].node_id, "coder");
  assert.equal(payload[0].behavior_id, "agent.delegate");
  assert.equal(payload[0].agent_ref, "agents/coder.md");
  assert.deepEqual(payload[0].depends_on_json, []);
  assert.equal(payload[0].join_mode, "all");
});
