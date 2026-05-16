import { describe, expect, it, vi } from "vitest";

import type { ContextSourceGateway } from "@/domain/gateway/context-source-gateway";
import type { FeatureGateway } from "@/domain/gateway/feature-gateway";
import type { ProjectGateway } from "@/domain/gateway/project-gateway";
import type { CreateContextSourcePayload } from "@/domain/model/payload/context-source-payload";
import { CreateContextSourceUseCase } from "./create-context-source-usecase";

function gateways(featureProjectId = "project_1") {
  const contextGateway = {
    createContextSource: vi.fn(async (payload: CreateContextSourcePayload) => ({
      id: "context_1",
      summarizedContent: null,
      createdBy: "test",
      createdAt: "2026-05-15T00:00:00.000Z",
      ...payload,
    })),
  } as unknown as ContextSourceGateway;
  const projectGateway = {
    getProjectById: vi.fn(async (projectId: string) => ({
      id: projectId,
      name: "Project",
      description: "Description",
      platform: "web",
      repositoryUrl: "https://example.com/repo",
      createdBy: "test",
      createdAt: "2026-05-15T00:00:00.000Z",
      updatedAt: "2026-05-15T00:00:00.000Z",
    })),
  } as unknown as ProjectGateway;
  const featureGateway = {
    getFeatureById: vi.fn(async (featureId: string) => ({
      id: featureId,
      projectId: featureProjectId,
      title: "Feature",
      businessGoal: "Goal",
      userProblem: "Problem",
      expectedFlow: "Flow",
      acceptanceCriteria: "Criteria",
      priority: "medium",
      status: "active",
      ownerId: "test",
      createdAt: "2026-05-15T00:00:00.000Z",
      updatedAt: "2026-05-15T00:00:00.000Z",
    })),
  } as unknown as FeatureGateway;

  return { contextGateway, projectGateway, featureGateway };
}

describe("CreateContextSourceUseCase", () => {
  it("creates project-level context when featureId is null", async () => {
    const deps = gateways();
    const result = await new CreateContextSourceUseCase(
      deps.contextGateway,
      deps.projectGateway,
      deps.featureGateway,
    ).execute({
      projectId: "project_1",
      featureId: null,
      type: "manual_text",
      title: "Notes",
      rawContent: "Context",
    });

    expect(result.featureId).toBeNull();
  });

  it("rejects feature-scoped context when the feature belongs to another project", async () => {
    const deps = gateways("project_2");

    await expect(
      new CreateContextSourceUseCase(
        deps.contextGateway,
        deps.projectGateway,
        deps.featureGateway,
      ).execute({
        projectId: "project_1",
        featureId: "feature_1",
        type: "manual_text",
        title: "Notes",
        rawContent: "Context",
      }),
    ).rejects.toThrow("Feature not found for selected project.");
  });
});
