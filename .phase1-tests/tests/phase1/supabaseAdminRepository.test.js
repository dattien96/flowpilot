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
}
class FakeSupabase {
    insertedProject = null;
    updatedProject = null;
    from(table) {
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
