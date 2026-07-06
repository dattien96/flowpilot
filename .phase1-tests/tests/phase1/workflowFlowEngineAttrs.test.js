"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const supabaseAdminRepository_1 = require("../../packages/flowpilot-client-core/src/data/supabaseAdminRepository");
// Minimal fluent fake for supabase-js's query builder, scoped to what
// saveWorkflow/cloneWorkflow actually call. Each table has its own canned
// response per terminal method (maybeSingle/single/then-as-promise via
// insert/delete), configured per test.
class FakeTable {
    responses;
    lastEqFilters = {};
    lastPayload;
    constructor(responses) {
        this.responses = responses;
    }
    select() {
        return this;
    }
    eq(column, value) {
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
    upsert(payload) {
        this.lastPayload = payload;
        return this;
    }
    insert(payload) {
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
    then(resolve, reject) {
        Promise.resolve(this.responses.write ?? { data: null, error: null }).then(resolve, reject);
    }
}
class FakeSupabase {
    tables = new Map();
    register(table, fake) {
        this.tables.set(table, fake);
        return fake;
    }
    from(table) {
        const fake = this.tables.get(table);
        if (!fake)
            throw new Error(`no fake registered for table ${table}`);
        return fake;
    }
}
function builtinWorkflowRow(overrides = {}) {
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
(0, node_test_1.default)("saveWorkflow rejects editing a non-editable (built-in) workflow", async () => {
    const supabase = new FakeSupabase();
    supabase.register("workflows", new FakeTable({ maybeSingle: { data: { editable: false }, error: null } }));
    const repo = new supabaseAdminRepository_1.SupabaseAdminRepository(supabase);
    await strict_1.default.rejects(() => repo.saveWorkflow({ id: "wf-builtin-1", name: "hacked name" }), /built-in template/);
});
(0, node_test_1.default)("saveWorkflow allows editing when editable=true", async () => {
    const supabase = new FakeSupabase();
    supabase.register("workflows", new FakeTable({
        maybeSingle: { data: { editable: true }, error: null },
        single: { data: builtinWorkflowRow({ id: "wf-user-1", is_builtin: false, editable: true, name: "My Flow" }), error: null },
    }));
    const repo = new supabaseAdminRepository_1.SupabaseAdminRepository(supabase);
    const saved = await repo.saveWorkflow({ id: "wf-user-1", name: "My Flow" });
    strict_1.default.equal(saved.name, "My Flow");
    strict_1.default.equal(saved.editable, true);
});
(0, node_test_1.default)("cloneWorkflow creates an editable, non-builtin copy referencing the source", async () => {
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
    workflowsTable.single = () => Promise.resolve({ data: workflowsResponses.shift(), error: null });
    supabase.register("workflows", workflowsTable);
    supabase.register("workflow_steps", new FakeTable({ list: { data: [], error: null } }));
    const repo = new supabaseAdminRepository_1.SupabaseAdminRepository(supabase);
    const cloned = await repo.cloneWorkflow("wf-builtin-1", "My Review Loop");
    strict_1.default.equal(cloned.isBuiltin, false);
    strict_1.default.equal(cloned.editable, true);
    strict_1.default.equal(cloned.clonedFrom, "wf-builtin-1");
    strict_1.default.equal(cloned.name, "My Review Loop");
});
// Regression test for BUG-NOTE-CP42 #18: saveWorkflow used to DELETE every
// existing workflow_steps row before inserting the replacements, so an
// insert failure permanently lost the old steps (the delete had already
// committed). This asserts the call ordering directly: insert must happen
// before delete, and — per the fix — a failed insert must never be followed
// by a delete call at all.
(0, node_test_1.default)("saveWorkflow inserts new steps before deleting superseded ones", async () => {
    const supabase = new FakeSupabase();
    supabase.register("workflows", new FakeTable({
        maybeSingle: { data: null, error: null },
        single: { data: builtinWorkflowRow({ id: "wf-1", is_builtin: false, editable: true }), error: null },
    }));
    const callOrder = [];
    const stepsTable = new FakeTable({ write: { data: [{ id: "step-1" }], error: null } });
    stepsTable.insert = function (payload) {
        callOrder.push("insert");
        this.lastPayload = payload;
        return this;
    };
    stepsTable.delete = function () {
        callOrder.push("delete");
        return this;
    };
    supabase.register("workflow_steps", stepsTable);
    const repo = new supabaseAdminRepository_1.SupabaseAdminRepository(supabase);
    await repo.saveWorkflow({
        id: "wf-1",
        name: "wf",
        steps: [{ stepType: "coding" }],
    });
    strict_1.default.deepEqual(callOrder, ["insert", "delete"]);
});
(0, node_test_1.default)("saveWorkflow does not delete existing steps when the insert fails", async () => {
    const supabase = new FakeSupabase();
    supabase.register("workflows", new FakeTable({
        maybeSingle: { data: null, error: null },
        single: { data: builtinWorkflowRow({ id: "wf-1", is_builtin: false, editable: true }), error: null },
    }));
    let sawDelete = false;
    const stepsTable = new FakeTable({ write: { data: null, error: new Error("simulated FK violation") } });
    stepsTable.delete = function () {
        sawDelete = true;
        return this;
    };
    supabase.register("workflow_steps", stepsTable);
    const repo = new supabaseAdminRepository_1.SupabaseAdminRepository(supabase);
    await strict_1.default.rejects(() => repo.saveWorkflow({
        id: "wf-1",
        name: "wf",
        steps: [{ stepType: "coding" }],
    }));
    strict_1.default.equal(sawDelete, false, "existing steps must not be deleted when the replacement insert failed");
});
// Task-189 (owner-confirmed 2026-07-06, = BUG-236 contract): node identity
// (nodeId/behaviorId/agentRef/dependsOn/joinMode/cohort) lives ONLY on
// StepDefinition — WorkflowStep/workflow_steps is a pure relation/order
// table and must never carry node data. This test used to be named
// "mapWorkflowStep round-trips the new flow-engine attrs" and asserted the
// OPPOSITE (that saveWorkflow wrote node_id/behavior_id/etc into the
// workflow_steps insert payload) — that was the design BUG-236 explicitly
// rejected. Rewritten to assert the corrected contract instead of the
// rejected one.
(0, node_test_1.default)("saveWorkflow never writes node-identity fields into workflow_steps (BUG-236)", async () => {
    const supabase = new FakeSupabase();
    supabase.register("workflows", new FakeTable({
        maybeSingle: { data: null, error: null },
        single: { data: builtinWorkflowRow({ id: "wf-1", is_builtin: false, editable: true }), error: null },
    }));
    const stepsTable = new FakeTable({ write: { data: null, error: null } });
    supabase.register("workflow_steps", stepsTable);
    const repo = new supabaseAdminRepository_1.SupabaseAdminRepository(supabase);
    await repo.saveWorkflow({
        id: "wf-1",
        name: "wf",
        steps: [{ stepType: "coding", orderIndex: 0, isEnabled: true, requiresApproval: false }],
    });
    const payload = stepsTable.lastPayload;
    strict_1.default.deepEqual(Object.keys(payload[0]).sort(), [
        "is_enabled",
        "order_index",
        "requires_approval",
        "step_type",
        "workflow_id",
    ]);
    strict_1.default.equal(payload[0].step_type, "coding");
    strict_1.default.equal(payload[0].order_index, 0);
    strict_1.default.equal(payload[0].is_enabled, true);
    strict_1.default.equal(payload[0].requires_approval, false);
    for (const nodeField of ["node_id", "behavior_id", "agent_ref", "depends_on_json", "join_mode", "cohort"]) {
        strict_1.default.equal(nodeField in payload[0], false, `workflow_steps payload must never carry "${nodeField}"`);
    }
});
