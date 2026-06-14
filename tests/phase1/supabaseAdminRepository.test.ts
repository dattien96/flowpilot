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
}

class FakeSupabase {
  public insertedProject: Record<string, unknown> | null = null;
  public updatedProject: Record<string, unknown> | null = null;

  from(table: string) {
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
