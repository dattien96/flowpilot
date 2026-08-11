import test from "node:test";
import assert from "node:assert/strict";

import { SupabaseAdminRepository } from "../../packages/flowpilot-client-core/src/data/supabaseAdminRepository";

// CP-55 P-1 pass-3 review fix (Task-263/CA-424): AcceptanceNodes was parsed
// and persisted by the Go-backed Supabase store (supabase_workflow_flow_store.go)
// but the desktop Settings UI talks to Supabase directly through
// SupabaseAdminRepository, which had no notion of `acceptanceNodes`/
// `acceptance_nodes_json` at all — so a built-in migrated later and cloned
// from Settings would have received the column's DB default `[]` and lost
// its declared safety boundary. These are additive tests only; no
// pre-existing test file is modified.
//
// Minimal fluent fake for supabase-js's query builder, scoped to what
// saveWorkflow/cloneWorkflow/listWorkflows actually call — mirrors the
// FakeTable/FakeSupabase pattern already used in
// tests/phase1/workflowFlowEngineAttrs.test.ts (duplicated locally per this
// test suite's existing per-file convention, not shared/exported).
class FakeTable {
  public lastPayload: unknown;
  public insertPayloads: unknown[] = [];

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
  eq() {
    return this;
  }
  neq() {
    return this;
  }
  in() {
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
    return this;
  }
  delete() {
    return this;
  }
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

function workflowRow(overrides: Record<string, unknown> = {}) {
  return {
    id: "wf-1",
    project_id: null,
    name: "My Flow",
    description: "",
    is_template: false,
    provider_override: null,
    model_override: null,
    reasoning_effort_override: null,
    yolo_mode: false,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    is_builtin: false,
    editable: true,
    cloneable: true,
    cloned_from: null,
    pack_id: null,
    pack_version: null,
    pack_flow_id: null,
    pack_hash: null,
    selectable_in_json: [],
    chat_baseline: false,
    chat_sub_modes_json: [],
    policy_cap: null,
    policy_on_cap: null,
    policy_extend_by: null,
    policy_extend_max: null,
    edges_json: [],
    ...overrides,
  };
}

test("listWorkflows maps acceptance_nodes_json into Workflow.acceptanceNodes", async () => {
  const supabase = new FakeSupabase();
  supabase.register(
    "workflows",
    new FakeTable({ list: { data: [workflowRow({ acceptance_nodes_json: ["validate", "audit"] })], error: null } }),
  );
  const repo = new SupabaseAdminRepository(supabase as any);

  const [workflow] = await repo.listWorkflows();

  assert.deepEqual(workflow.acceptanceNodes, ["validate", "audit"]);
});

test("listWorkflows defaults acceptanceNodes to [] for a legacy row with no acceptance_nodes_json", async () => {
  const supabase = new FakeSupabase();
  supabase.register("workflows", new FakeTable({ list: { data: [workflowRow()], error: null } }));
  const repo = new SupabaseAdminRepository(supabase as any);

  const [workflow] = await repo.listWorkflows();

  assert.deepEqual(workflow.acceptanceNodes, []);
});

test("saveWorkflow sends the caller's acceptanceNodes as acceptance_nodes_json for an editable workflow", async () => {
  const supabase = new FakeSupabase();
  const workflowsTable = new FakeTable({
    maybeSingle: { data: { editable: true }, error: null },
    single: { data: workflowRow({ acceptance_nodes_json: ["validate"] }), error: null },
  });
  supabase.register("workflows", workflowsTable);
  const repo = new SupabaseAdminRepository(supabase as any);

  const saved = await repo.saveWorkflow({ id: "wf-1", name: "My Flow", acceptanceNodes: ["validate"] });

  const sent = workflowsTable.lastPayload as Record<string, unknown>;
  assert.deepEqual(sent.acceptance_nodes_json, ["validate"]);
  assert.deepEqual(saved.acceptanceNodes, ["validate"]);
});

test("saveWorkflow sends an empty array (never null/undefined) when acceptanceNodes is omitted", async () => {
  const supabase = new FakeSupabase();
  const workflowsTable = new FakeTable({
    maybeSingle: { data: { editable: true }, error: null },
    single: { data: workflowRow(), error: null },
  });
  supabase.register("workflows", workflowsTable);
  const repo = new SupabaseAdminRepository(supabase as any);

  await repo.saveWorkflow({ id: "wf-1", name: "My Flow" });

  const sent = workflowsTable.lastPayload as Record<string, unknown>;
  assert.deepEqual(sent.acceptance_nodes_json, []);
  assert.notEqual(sent.acceptance_nodes_json, null);
});

// Regression guard: Settings can only ever change model/reasoning/yolo
// overrides for a built-in (editable=false) workflow — acceptance_nodes_json
// for a built-in is Go-mirror-sync-owned. Even if a caller passed
// acceptanceNodes for a built-in save, the override payload must not carry
// acceptance_nodes_json at all, exactly like it never carries name/edges.
test("saveWorkflow never sends acceptance_nodes_json through the built-in override path", async () => {
  const supabase = new FakeSupabase();
  const workflowsTable = new FakeTable({
    maybeSingle: { data: { editable: false }, error: null },
    single: { data: workflowRow({ is_builtin: true, editable: false }), error: null },
  });
  let updatePayload: Record<string, unknown> | null = null;
  (workflowsTable as any).update = function (payload: Record<string, unknown>) {
    updatePayload = payload;
    return this;
  };
  supabase.register("workflows", workflowsTable);
  const repo = new SupabaseAdminRepository(supabase as any);

  await repo.saveWorkflow({
    id: "wf-1",
    modelOverride: "gpt-5.4",
    reasoningEffortOverride: "high",
    yoloMode: true,
    acceptanceNodes: ["should-not-be-sent"],
  });

  assert.deepEqual(Object.keys(updatePayload as any).sort(), [
    "model_override",
    "reasoning_effort_override",
    "updated_at",
    "yolo_mode",
  ]);
});

// The core CP-55 P-1 pass-3 fix: cloning must deep-copy the declared
// acceptance boundary exactly like it already deep-copies edges_json,
// instead of falling back to the acceptance_nodes_json column's DB default
// (`[]`) the way a plain INSERT with the column omitted would.
test("cloneWorkflow preserves the source workflow's acceptance_nodes_json onto the clone", async () => {
  const supabase = new FakeSupabase();
  const sourceRow = workflowRow({ id: "wf-builtin-1", is_builtin: true, editable: false, acceptance_nodes_json: ["validate"] });
  const clonedRow = workflowRow({
    id: "wf-clone-1",
    is_builtin: false,
    editable: true,
    cloned_from: "wf-builtin-1",
    name: "My Clone",
    acceptance_nodes_json: ["validate"],
  });
  const workflowsResponses = [sourceRow, clonedRow];
  const workflowsTable = new FakeTable({});
  (workflowsTable as any).single = () => Promise.resolve({ data: workflowsResponses.shift(), error: null });
  supabase.register("workflows", workflowsTable);
  supabase.register("workflow_steps", new FakeTable({ list: { data: [], error: null } }));

  const repo = new SupabaseAdminRepository(supabase as any);
  const cloned = await repo.cloneWorkflow("wf-builtin-1", "My Clone");

  const insertPayload = workflowsTable.insertPayloads[0] as Record<string, unknown>;
  assert.deepEqual(insertPayload.acceptance_nodes_json, ["validate"]);
  assert.deepEqual(cloned.acceptanceNodes, ["validate"]);
});

test("cloneWorkflow sends an empty array when the source declares no acceptance nodes", async () => {
  const supabase = new FakeSupabase();
  const sourceRow = workflowRow({ id: "wf-builtin-1", is_builtin: true, editable: false });
  const clonedRow = workflowRow({ id: "wf-clone-1", is_builtin: false, editable: true, cloned_from: "wf-builtin-1" });
  const workflowsResponses = [sourceRow, clonedRow];
  const workflowsTable = new FakeTable({});
  (workflowsTable as any).single = () => Promise.resolve({ data: workflowsResponses.shift(), error: null });
  supabase.register("workflows", workflowsTable);
  supabase.register("workflow_steps", new FakeTable({ list: { data: [], error: null } }));

  const repo = new SupabaseAdminRepository(supabase as any);
  await repo.cloneWorkflow("wf-builtin-1", "My Clone");

  const insertPayload = workflowsTable.insertPayloads[0] as Record<string, unknown>;
  assert.deepEqual(insertPayload.acceptance_nodes_json, []);
  assert.notEqual(insertPayload.acceptance_nodes_json, null);
});
