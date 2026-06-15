"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const supabaseAdminRepository_1 = require("../../packages/flowpilot-client-core/src/data/supabaseAdminRepository");
class FakeQuery {
    runner;
    constructor(runner) {
        this.runner = runner;
    }
    select() {
        return this;
    }
    single() {
        return this.runner();
    }
    order() {
        return this.runner();
    }
    eq() {
        return this;
    }
    neq() {
        return this;
    }
    delete() {
        return this.runner();
    }
    insert() {
        return this.runner();
    }
    upsert() {
        return this;
    }
}
class FakeSupabase {
    insertedProject = null;
    updatedProject = null;
    touchedTables = [];
    from(table) {
        this.touchedTables.push(table);
        if (table !== "projects") {
            throw new Error(`unexpected table ${table}`);
        }
        return {
            insert: (payload) => {
                this.insertedProject = payload;
                return new FakeQuery(async () => ({
                    data: {
                        id: "project-1",
                        name: payload.name,
                        description: payload.description,
                        platform: payload.platform,
                        repository_url: payload.repository_url,
                        directory_path: payload.directory_path,
                        status: payload.status,
                        artifact_storage_preference: payload.artifact_storage_preference,
                        default_provider: payload.default_provider,
                        default_model: payload.default_model,
                        default_reasoning_effort: payload.default_reasoning_effort,
                        session_idle_ttl_minutes: payload.session_idle_ttl_minutes,
                        created_at: "",
                        updated_at: "",
                    },
                    error: null,
                }));
            },
            update: (payload) => {
                this.updatedProject = payload;
                return {
                    eq: () => new FakeQuery(async () => ({
                        data: {
                            id: "project-1",
                            name: "Project",
                            description: "",
                            platform: "android",
                            repository_url: "",
                            directory_path: "",
                            status: "active",
                            artifact_storage_preference: "supabase",
                            default_provider: null,
                            default_model: null,
                            default_reasoning_effort: null,
                            session_idle_ttl_minutes: payload.session_idle_ttl_minutes,
                            created_at: "",
                            updated_at: "",
                        },
                        error: null,
                    })),
                };
            },
            select: () => new FakeQuery(async () => ({ data: [], error: null })),
        };
    }
}
(0, node_test_1.default)("SupabaseAdminRepository saves project session idle TTL on create", async () => {
    const supabase = new FakeSupabase();
    const repository = new supabaseAdminRepository_1.SupabaseAdminRepository(supabase);
    const project = await repository.createProject({
        name: "Project",
        description: "",
        platform: "android",
        repositoryUrl: "",
        sessionIdleTtlMinutes: 45,
    });
    strict_1.default.equal(project.sessionIdleTtlMinutes, 45);
    strict_1.default.equal(supabase.insertedProject?.session_idle_ttl_minutes, 45);
});
(0, node_test_1.default)("SupabaseAdminRepository saves project session idle TTL on update", async () => {
    const supabase = new FakeSupabase();
    const repository = new supabaseAdminRepository_1.SupabaseAdminRepository(supabase);
    const project = await repository.updateProject("project-1", {
        sessionIdleTtlMinutes: 90,
    });
    strict_1.default.equal(project.sessionIdleTtlMinutes, 90);
    strict_1.default.equal(supabase.updatedProject?.session_idle_ttl_minutes, 90);
});
(0, node_test_1.default)("SupabaseAdminRepository reads step definitions from the migrated step tables", async () => {
    const touchedTables = [];
    const supabase = {
        from(table) {
            touchedTables.push(table);
            if (table === "step_definitions") {
                return new FakeQuery(async () => ({
                    data: [{
                            step_type: "tech_spec",
                            name: "Tech Spec",
                            description: "Describe the technical solution",
                            prompt_base: null,
                            required_mcps: ["jira"],
                            mcp_access_mode: "read_only",
                            required_skills: ["tech_spec_skill"],
                            team_role: null,
                            subagent: null,
                            model: "gpt-5.4",
                            reasoning_effort: "medium",
                            yolo_mode: false,
                            agent_type: "standard",
                            created_at: "",
                            updated_at: "",
                        }],
                    error: null,
                }));
            }
            if (table === "step_input_artifact_definitions") {
                return new FakeQuery(async () => ({
                    data: [{ step_type: "tech_spec", artifact_definition_key: "prd", order_index: 0 }],
                    error: null,
                }));
            }
            if (table === "step_output_artifact_definitions") {
                return new FakeQuery(async () => ({
                    data: [{ step_type: "tech_spec", artifact_definition_key: "spec", order_index: 0 }],
                    error: null,
                }));
            }
            throw new Error(`unexpected table ${table}`);
        },
    };
    const repository = new supabaseAdminRepository_1.SupabaseAdminRepository(supabase);
    const [definition] = await repository.listStepDefinitions();
    strict_1.default.equal(definition.stepType, "tech_spec");
    strict_1.default.deepEqual(definition.inputArtifactDefinitions, ["prd"]);
    strict_1.default.deepEqual(definition.outputArtifactDefinitions, ["spec"]);
    strict_1.default.deepEqual(touchedTables, [
        "step_definitions",
        "step_input_artifact_definitions",
        "step_output_artifact_definitions",
    ]);
});
(0, node_test_1.default)("SupabaseAdminRepository lists only definition workflows, not runtime-generated rows", async () => {
    const filters = [];
    const supabase = {
        from(table) {
            if (table !== "workflows") {
                throw new Error(`unexpected table ${table}`);
            }
            return {
                select() {
                    return this;
                },
                neq(column, value) {
                    filters.push({ operator: "neq", column, value });
                    return this;
                },
                order() {
                    return Promise.resolve({
                        data: [{
                                id: "workflow-1",
                                project_id: "project-1",
                                name: "Definition Workflow",
                                description: "Reusable flow",
                                is_template: false,
                                provider_override: null,
                                model_override: null,
                                reasoning_effort_override: null,
                                yolo_mode: false,
                                created_at: "",
                                updated_at: "",
                            }],
                        error: null,
                    });
                },
            };
        },
    };
    const repository = new supabaseAdminRepository_1.SupabaseAdminRepository(supabase);
    const [workflow] = await repository.listWorkflows();
    strict_1.default.equal(workflow.id, "workflow-1");
    strict_1.default.deepEqual(filters, [
        { operator: "neq", column: "created_by", value: "flowpilot-runtime" },
    ]);
});
(0, node_test_1.default)("SupabaseAdminRepository reads linked integrations from project_mcp_links", async () => {
    const touchedTables = [];
    const supabase = {
        from(table) {
            touchedTables.push(table);
            if (table === "project_mcp_links") {
                return {
                    select() {
                        return this;
                    },
                    eq() {
                        return Promise.resolve({
                            data: [{
                                    integration_id: "integration-1",
                                    integrations: {
                                        id: "integration-1",
                                        project_id: "project-1",
                                        type: "jira",
                                        label: "Jira",
                                        mcp_type_enabled: true,
                                        config_encrypted: {},
                                        status: "connected",
                                        last_synced_at: null,
                                        last_error: null,
                                        created_at: "",
                                        updated_at: "",
                                    },
                                }],
                            error: null,
                        });
                    },
                };
            }
            throw new Error(`unexpected table ${table}`);
        },
    };
    const repository = new supabaseAdminRepository_1.SupabaseAdminRepository(supabase);
    const [integration] = await repository.listLinkedIntegrations("project-1");
    strict_1.default.equal(integration.id, "integration-1");
    strict_1.default.equal(integration.type, "jira");
    strict_1.default.deepEqual(touchedTables, ["project_mcp_links"]);
});
