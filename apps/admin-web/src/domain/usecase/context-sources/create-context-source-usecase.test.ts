import { describe, expect, it, vi } from "vitest";

import type { ContextSourceGateway } from "@/domain/gateway/context-source-gateway";
import type { ProjectGateway } from "@/domain/gateway/project-gateway";
import type { CreateContextSourcePayload } from "@/domain/model/payload/context-source-payload";
import { CreateContextSourceUseCase } from "./create-context-source-usecase";

function gateways() {
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

  return { contextGateway, projectGateway };
}

describe("CreateContextSourceUseCase", () => {
  it("creates project-level context", async () => {
    const deps = gateways();
    const result = await new CreateContextSourceUseCase(
      deps.contextGateway,
      deps.projectGateway,
    ).execute({
      projectId: "project_1",
      type: "manual_text",
      title: "Notes",
      rawContent: "Context",
    });

    expect(result.projectId).toBe("project_1");
  });

  it("rejects context creation when the project is missing", async () => {
    const deps = gateways();
    vi.spyOn(deps.projectGateway, "getProjectById").mockResolvedValueOnce(null);

    await expect(
      new CreateContextSourceUseCase(
        deps.contextGateway,
        deps.projectGateway,
      ).execute({
        projectId: "project_1",
        type: "manual_text",
        title: "Notes",
        rawContent: "Context",
      }),
    ).rejects.toThrow("Project not found.");
  });
});
