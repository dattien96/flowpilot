import { afterEach, describe, expect, it } from "vitest";

import { createDemoGatewayBundle } from "./demo-gateway-bundle";
import { demoIntegrations } from "./demo-store";

const integrationsSnapshot = structuredClone(demoIntegrations);

afterEach(() => {
  demoIntegrations.splice(0, demoIntegrations.length, ...structuredClone(integrationsSnapshot));
});

describe("DemoGatewayBundle integrations", () => {
  it("lists only integrations belonging to the requested project", async () => {
    const gateway = createDemoGatewayBundle().integrationGateway;

    const integrations = await gateway.listIntegrationsByProject("project_meal_suggestion");

    expect(integrations).toHaveLength(2);
    expect(integrations.every((integration) => integration.projectId === "project_meal_suggestion")).toBe(true);
  });

  it("creates a pending integration with label and config", async () => {
    const gateway = createDemoGatewayBundle().integrationGateway;

    const created = await gateway.createIntegration({
      projectId: "project_meal_suggestion",
      type: "figma",
      label: "Design System",
      configEncrypted: { fileKey: "abc123" },
    });

    expect(created.status).toBe("pending");
    expect(created.lastError).toBeNull();
    expect(demoIntegrations.some((integration) => integration.id === created.id)).toBe(true);
  });

  it("updates integration label, status, and error fields", async () => {
    const gateway = createDemoGatewayBundle().integrationGateway;
    const original = demoIntegrations[0];

    const updated = await gateway.updateIntegration(original.id, {
      label: "Updated Label",
      status: "failed",
      lastError: "runner offline",
    });

    expect(updated.label).toBe("Updated Label");
    expect(updated.status).toBe("failed");
    expect(updated.lastError).toBe("runner offline");
    expect(updated.updatedAt).not.toBe(original.createdAt);
  });

  it("clears lastError when a retry resets the integration back to pending", async () => {
    const gateway = createDemoGatewayBundle().integrationGateway;
    const target = demoIntegrations.find((integration) => integration.status === "failed");
    if (!target) {
      throw new Error("Expected a failed seeded integration.");
    }

    const updated = await gateway.updateIntegration(target.id, {
      status: "pending",
    });

    expect(updated.status).toBe("pending");
    expect(updated.lastError).toBeNull();
  });

  it("deletes an integration by id", async () => {
    const gateway = createDemoGatewayBundle().integrationGateway;
    const targetId = demoIntegrations[0].id;

    await gateway.deleteIntegration(targetId);

    expect(demoIntegrations.some((integration) => integration.id === targetId)).toBe(false);
  });
});
