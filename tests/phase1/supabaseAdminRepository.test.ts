import test from "node:test";
import assert from "node:assert/strict";

import { SupabaseAdminRepository } from "../../packages/flowpilot-client-core/src/data/supabaseAdminRepository";

class FakeQuery {
  constructor(
    private readonly runner: () => Promise<{ data?: any; error?: { message?: string } | null }>,
  ) {}

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
  public insertedProject: Record<string, unknown> | null = null;
  public updatedProject: Record<string, unknown> | null = null;
  public touchedTables: string[] = [];

  from(table: string) {
    this.touchedTables.push(table);
    if (table !== "projects") {
      throw new Error(`unexpected table ${table}`);
    }
    return {
      insert: (payload: Record<string, unknown>) => {
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
      update: (payload: Record<string, unknown>) => {
        this.updatedProject = payload;
        return {
          eq: () =>
            new FakeQuery(async () => ({
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

test("SupabaseAdminRepository saves project session idle TTL on create", async () => {
  const supabase = new FakeSupabase();
  const repository = new SupabaseAdminRepository(supabase as never);

  const project = await repository.createProject({
    name: "Project",
    description: "",
    platform: "android",
    repositoryUrl: "",
    sessionIdleTtlMinutes: 45,
  });

  assert.equal(project.sessionIdleTtlMinutes, 45);
  assert.equal(supabase.insertedProject?.session_idle_ttl_minutes, 45);
});

test("SupabaseAdminRepository saves project session idle TTL on update", async () => {
  const supabase = new FakeSupabase();
  const repository = new SupabaseAdminRepository(supabase as never);

  const project = await repository.updateProject("project-1", {
    sessionIdleTtlMinutes: 90,
  });

  assert.equal(project.sessionIdleTtlMinutes, 90);
  assert.equal(supabase.updatedProject?.session_idle_ttl_minutes, 90);
});

test("SupabaseAdminRepository reads step definitions with their CP-45 artifact bindings", async () => {
  const touchedTables: string[] = [];
  const supabase = {
    from(table: string) {
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
      if (table === "step_artifact_bindings") {
        return new FakeQuery(async () => ({
          data: [{
            id: "binding-1",
            step_definition_id: "tech_spec",
            direction: "input",
            slot_name: "",
            artifact_instance_id: "instance-1",
            required: true,
            position: 0,
            created_at: "",
          }],
          error: null,
        }));
      }
      throw new Error(`unexpected table ${table}`);
    },
  };

  const repository = new SupabaseAdminRepository(supabase as never);
  const [definition] = await repository.listStepDefinitions();

  assert.equal(definition.stepType, "tech_spec");
  assert.equal(definition.artifactBindings.length, 1);
  assert.equal(definition.artifactBindings[0].artifactInstanceId, "instance-1");
  assert.deepEqual(touchedTables, [
    "step_definitions",
    "step_artifact_bindings",
  ]);
});

test("SupabaseAdminRepository lists only definition workflows, not runtime-generated rows", async () => {
  const filters: Array<{ operator: string; column: string; value: string }> = [];
  const supabase = {
    from(table: string) {
      if (table !== "workflows") {
        throw new Error(`unexpected table ${table}`);
      }
      return {
        select() {
          return this;
        },
        neq(column: string, value: string) {
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

  const repository = new SupabaseAdminRepository(supabase as never);
  const [workflow] = await repository.listWorkflows();

  assert.equal(workflow.id, "workflow-1");
  assert.deepEqual(filters, [
    { operator: "neq", column: "created_by", value: "flowpilot-runtime" },
  ]);
});

test("SupabaseAdminRepository reads linked integrations from project_mcp_links", async () => {
  const touchedTables: string[] = [];
  const supabase = {
    from(table: string) {
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

  const repository = new SupabaseAdminRepository(supabase as never);
  const [integration] = await repository.listLinkedIntegrations("project-1");

  assert.equal(integration.id, "integration-1");
  assert.equal(integration.type, "jira");
  assert.deepEqual(touchedTables, ["project_mcp_links"]);
});
