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
    let insertedPayload: Record<string, unknown> | null = null;
    let linkedPayload: Record<string, unknown> | null = null;
    const single = vi.fn().mockResolvedValue({
      data: buildIntegrationRow({
        id: "integration-created",
        status: "pending",
      }),
      error: null,
    });
    const select = vi.fn(() => ({ single }));
    const insert = vi.fn((payload: Record<string, unknown>) => {
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
    const upsert = vi.fn((payload: Record<string, unknown>) => {
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
    let updatePayload: Record<string, unknown> | null = null;
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
    const update = vi.fn((payload: Record<string, unknown>) => {
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
