import { describe, expect, it, vi } from "vitest";

import { createSupabaseGatewayBundle } from "./supabase-gateway-bundle";

function buildIntegrationRow(overrides: Record<string, unknown> = {}) {
  return {
    id: "integration-1",
    project_id: "project-alpha",
    type: "jira",
    label: "Alpha Jira",
    config_encrypted: { workspaceUrl: "https://jira.example.com" },
    status: "connected",
    last_synced_at: "2026-05-19T08:00:00.000Z",
    last_error: null,
    created_at: "2026-05-19T08:00:00.000Z",
    updated_at: "2026-05-19T08:00:00.000Z",
    ...overrides,
  };
}

function buildProjectRow(overrides: Record<string, unknown> = {}) {
  return {
    id: "project-alpha",
    name: "Alpha Project",
    description: "Alpha description",
    platform: "web",
    repository_url: "https://example.com/repo.git",
    directory_path: "/workspace/alpha",
    owner_id: null,
    status: "active",
    artifact_storage_preference: "supabase",
    default_provider: "codex",
    default_model: "gpt-5.5",
    default_reasoning_effort: "high",
    created_by: "demo-user",
    created_at: "2026-05-20T00:00:00.000Z",
    updated_at: "2026-05-20T00:00:00.000Z",
    ...overrides,
  };
}

function buildBindingRow(overrides: Record<string, unknown> = {}) {
  return {
    id: "binding-1",
    project_id: "project-alpha",
    local_path: "/workspace/alpha",
    label: "Primary",
    created_at: "2026-05-20T00:00:00.000Z",
    updated_at: "2026-05-20T00:00:00.000Z",
    ...overrides,
  };
}

describe("SupabaseGatewayBundle integrations", () => {
  it("maps integration rows including label, awaiting_oauth, and last_error", async () => {
    const order = vi.fn().mockResolvedValue({
      data: [
        buildIntegrationRow({
          status: "awaiting_oauth",
          last_synced_at: null,
          last_error: "oauth approval pending",
        }),
      ],
      error: null,
    });
    const eq = vi.fn(() => ({ order }));
    const select = vi.fn(() => ({ eq }));
    const from = vi.fn(() => ({ select }));

    const gateway = createSupabaseGatewayBundle({ from } as never).integrationGateway;
    const [integration] = await gateway.listIntegrationsByProject("project-alpha");

    expect(from).toHaveBeenCalledWith("integrations");
    expect(eq).toHaveBeenCalledWith("project_id", "project-alpha");
    expect(integration).toMatchObject({
      label: "Alpha Jira",
      status: "awaiting_oauth",
      lastError: "oauth approval pending",
      lastSyncedAt: null,
    });
  });

  it("inserts the expected integration payload for create", async () => {
    let insertedPayload: any = null;
    let linkedPayload: any = null;
    const single = vi.fn().mockResolvedValue({
      data: buildIntegrationRow({
        id: "integration-created",
        status: "pending",
      }),
      error: null,
    });
    const select = vi.fn(() => ({ single }));
    const insert = vi.fn((payload: Record<string, any>) => {
      insertedPayload = payload;
      return { select };
    });
    const selectIntegration = vi.fn(() => ({
      eq: vi.fn(() => ({
        single: vi.fn().mockResolvedValue({
          data: { id: "integration-created", type: "google_drive" },
          error: null,
        }),
      })),
    }));
    const upsert = vi.fn((payload: Record<string, any>) => {
      linkedPayload = payload;
      return Promise.resolve({ error: null });
    });
    const from = vi.fn((table: string) => {
      if (table === "integrations") {
        return {
          insert,
          select: selectIntegration,
        };
      }
      if (table === "project_mcp_links") {
        return {
          upsert,
        };
      }
      throw new Error(`Unexpected table ${table}`);
    });

    const gateway = createSupabaseGatewayBundle({ from } as never).integrationGateway;
    const integration = await gateway.createIntegration({
      projectId: "project-alpha",
      type: "google_drive",
      label: "Project Drive",
      configEncrypted: { folderId: "folder-1" },
      status: "pending",
    });

    expect(from).toHaveBeenCalledWith("integrations");
    expect(insertedPayload).toMatchObject({
      project_id: "project-alpha",
      type: "google_drive",
      label: "Project Drive",
      config_encrypted: { folderId: "folder-1" },
      status: "pending",
      last_error: null,
    });
    expect(typeof insertedPayload?.id).toBe("string");
    expect(linkedPayload).toMatchObject({
      project_id: "project-alpha",
      integration_id: "integration-created",
      type: "google_drive",
    });
    expect(integration.id).toBe("integration-created");
  });

  it("updates mutable fields and stamps updated_at", async () => {
    let updatePayload: any = null;
    const single = vi.fn().mockResolvedValue({
      data: buildIntegrationRow({
        label: "Updated Jira",
        config_encrypted: { board: "ALPHA" },
        status: "failed",
        last_error: "runner offline",
      }),
      error: null,
    });
    const select = vi.fn(() => ({ single }));
    const eq = vi.fn(() => ({ select }));
    const update = vi.fn((payload: Record<string, any>) => {
      updatePayload = payload;
      return { eq };
    });
    const from = vi.fn(() => ({ update }));

    const gateway = createSupabaseGatewayBundle({ from } as never).integrationGateway;
    const integration = await gateway.updateIntegration("integration-1", {
      type: "jira",
      label: "Updated Jira",
      configEncrypted: { board: "ALPHA" },
      status: "failed",
      lastError: "runner offline",
    });

    expect(from).toHaveBeenCalledWith("integrations");
    expect(eq).toHaveBeenCalledWith("id", "integration-1");
    expect(updatePayload).toMatchObject({
      type: "jira",
      label: "Updated Jira",
      config_encrypted: { board: "ALPHA" },
      status: "failed",
      last_error: "runner offline",
    });
    expect(typeof updatePayload?.updated_at).toBe("string");
    expect(integration.lastError).toBe("runner offline");
  });

  it("deletes the integration row by id", async () => {
    const eq = vi.fn().mockResolvedValue({ error: null });
    const del = vi.fn(() => ({ eq }));
    const from = vi.fn(() => ({ delete: del }));

    const gateway = createSupabaseGatewayBundle({ from } as never).integrationGateway;
    await gateway.deleteIntegration("integration-1");

    expect(from).toHaveBeenCalledWith("integrations");
    expect(eq).toHaveBeenCalledWith("id", "integration-1");
  });
});

describe("SupabaseGatewayBundle workflow runs", () => {
  it("deletes workflow runs by id list", async () => {
    const workflowRunsIn = vi.fn().mockResolvedValue({ error: null });
    const workflowRunsDelete = vi.fn(() => ({ in: workflowRunsIn }));
    const from = vi.fn((table: string) => {
      if (table === "workflow_runs") {
        return { delete: workflowRunsDelete };
      }
      throw new Error(`Unexpected table ${table}`);
    });

    const gateway = createSupabaseGatewayBundle({
      from,
      storage: {
        from: vi.fn(),
      },
    } as never).workflowGateway;
    await gateway.deleteWorkflowRuns(["run-1", "run-2"]);

    expect(from).toHaveBeenCalledWith("workflow_runs");
    expect(workflowRunsIn).toHaveBeenCalledWith("id", ["run-1", "run-2"]);
  });

  it("retains remote storage objects when deleting workflow runs", async () => {
    const workflowRunsIn = vi.fn().mockResolvedValue({ error: null });
    const workflowRunsDelete = vi.fn(() => ({ in: workflowRunsIn }));
    const storageFrom = vi.fn();
    const from = vi.fn((table: string) => {
      if (table === "workflow_runs") {
        return { delete: workflowRunsDelete };
      }
      throw new Error(`Unexpected table ${table}`);
    });

    const gateway = createSupabaseGatewayBundle({
      from,
      storage: {
        from: storageFrom,
      },
    } as never).workflowGateway;
    await gateway.deleteWorkflowRuns(["run-1"]);

    expect(storageFrom).not.toHaveBeenCalled();
    expect(workflowRunsIn).toHaveBeenCalledWith("id", ["run-1"]);
  });
});

describe("SupabaseGatewayBundle project workspace bindings", () => {
  it("lists bindings ordered by creation time", async () => {
    const order = vi.fn().mockResolvedValue({
      data: [
        buildBindingRow({
          id: "binding-1",
          local_path: "/workspace/alpha",
          created_at: "2026-05-20T00:00:00.000Z",
        }),
        buildBindingRow({
          id: "binding-2",
          local_path: "/workspace/beta",
          created_at: "2026-05-21T00:00:00.000Z",
        }),
      ],
      error: null,
    });
    const eq = vi.fn(() => ({ order }));
    const select = vi.fn(() => ({ eq }));
    const from = vi.fn(() => ({ select }));

    const gateway = createSupabaseGatewayBundle({ from } as never).projectGateway;
    const bindings = await gateway.listProjectWorkspaceBindings("project-alpha");

    expect(from).toHaveBeenCalledWith("project_workspace_bindings");
    expect(eq).toHaveBeenCalledWith("project_id", "project-alpha");
    expect(bindings).toEqual([
      expect.objectContaining({
        id: "binding-1",
        localPath: "/workspace/alpha",
        label: "Primary",
      }),
      expect.objectContaining({
        id: "binding-2",
        localPath: "/workspace/beta",
      }),
    ]);
  });

  it("creates a project and seeds the primary binding", async () => {
    let projectInsertPayload: any = null;
    let bindingInsertPayload: any = null;
    const projectSingle = vi.fn().mockResolvedValue({
      data: buildProjectRow({ id: "project-created", directory_path: "/workspace/alpha" }),
      error: null,
    });
    const bindingSingle = vi.fn().mockResolvedValue({
      data: buildBindingRow({
        id: "binding-created",
        project_id: "project-created",
        local_path: "/workspace/alpha",
        label: "Primary",
      }),
      error: null,
    });
    const projectSelect = vi.fn(() => ({ single: projectSingle }));
    const bindingSelect = vi.fn(() => ({ single: bindingSingle }));
    const projectInsert = vi.fn((payload: Record<string, any>) => {
      projectInsertPayload = payload;
      return { select: projectSelect };
    });
    const bindingInsert = vi.fn((payload: Record<string, any>) => {
      bindingInsertPayload = payload;
      return { select: bindingSelect };
    });
    const from = vi.fn((table: string) => {
      if (table === "projects") {
        return { insert: projectInsert };
      }
      if (table === "project_workspace_bindings") {
        return { insert: bindingInsert };
      }
      if (table === "ai_supported_models") {
        return {
          select: vi.fn(() => ({
            eq: vi.fn(() => ({
              maybeSingle: vi.fn().mockResolvedValue({ data: null, error: null }),
            })),
          })),
        };
      }
      throw new Error(`Unexpected table ${table}`);
    });

    const gateway = createSupabaseGatewayBundle({ from } as never).projectGateway;
    const project = await gateway.createProject({
      name: "Alpha Project",
      description: "Alpha description",
      platform: "web",
      repositoryUrl: "https://example.com/repo.git",
      directoryPath: "  /workspace/alpha  ",
      status: "active",
      artifactStoragePreference: "supabase",
      defaultReasoningEffort: "medium",
    });

    expect(projectInsertPayload).toMatchObject({
      legacy_id: expect.stringMatching(/^project_[0-9a-f]{18}$/i),
      name: "Alpha Project",
      description: "Alpha description",
      platform: "web",
      repository_url: "https://example.com/repo.git",
      directory_path: "/workspace/alpha",
      default_provider: "codex",
      default_model: "gpt-5.4",
      default_reasoning_effort: "medium",
      created_by: "supabase-admin",
    });
    expect(projectInsertPayload?.id).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i,
    );
    expect(bindingInsertPayload).toMatchObject({
      project_id: "project-created",
      local_path: "/workspace/alpha",
      label: "Primary",
    });
    expect(project.id).toBe("project-created");
  });

  it("updates and deletes a project binding", async () => {
    let updatePayload: any = null;
    const updateSingle = vi.fn().mockResolvedValue({
      data: buildBindingRow({
        id: "binding-1",
        local_path: "/workspace/beta",
        label: "Secondary",
      }),
      error: null,
    });
    const updateSelect = vi.fn(() => ({ single: updateSingle }));
    const updateEq = vi.fn(() => ({ select: updateSelect }));
    const update = vi.fn((payload: Record<string, any>) => {
      updatePayload = payload;
      return { eq: updateEq };
    });
    const deleteEq = vi.fn().mockResolvedValue({ error: null });
    const del = vi.fn(() => ({ eq: deleteEq }));
    const from = vi.fn((table: string) => {
      if (table === "project_workspace_bindings") {
        return { update, delete: del };
      }
      throw new Error(`Unexpected table ${table}`);
    });

    const gateway = createSupabaseGatewayBundle({ from } as never).projectGateway;
    const updated = await gateway.updateProjectWorkspaceBinding("binding-1", {
      localPath: "  /workspace/beta  ",
      label: "  Secondary  ",
    });
    await gateway.deleteProjectWorkspaceBinding("binding-1");

    expect(updatePayload).toMatchObject({
      local_path: "/workspace/beta",
      label: "Secondary",
    });
    expect(typeof updatePayload?.updated_at).toBe("string");
    expect(updated.localPath).toBe("/workspace/beta");
    expect(deleteEq).toHaveBeenCalledWith("id", "binding-1");
  });
});

describe("SupabaseGatewayBundle project team links", () => {
  it("writes both project_id and legacy_project_id when linking a team", async () => {
    let insertedPayload: any = null;
    const projectSingle = vi.fn().mockResolvedValue({
      data: { legacy_id: "project_legacy_alpha" },
      error: null,
    });
    const projectEq = vi.fn(() => ({ single: projectSingle }));
    const projectSelect = vi.fn(() => ({ eq: projectEq }));
    const insert = vi.fn((payload: Record<string, any>) => {
      insertedPayload = payload;
      return Promise.resolve({ error: null });
    });
    const from = vi.fn((table: string) => {
      if (table === "projects") {
        return { select: projectSelect };
      }
      if (table === "project_teams") {
        return { insert };
      }
      throw new Error(`Unexpected table ${table}`);
    });

    const gateway = createSupabaseGatewayBundle({ from } as never).teamGateway;
    await gateway.linkTeamToProject("3f1b7795-4f38-4bdd-a8c7-f0c576c3f218", "team-1");

    expect(projectEq).toHaveBeenCalledWith("id", "3f1b7795-4f38-4bdd-a8c7-f0c576c3f218");
    expect(insertedPayload).toMatchObject({
      project_id: "3f1b7795-4f38-4bdd-a8c7-f0c576c3f218",
      legacy_project_id: "project_legacy_alpha",
      team_id: "team-1",
    });
    expect(typeof insertedPayload?.id).toBe("string");
  });
});
