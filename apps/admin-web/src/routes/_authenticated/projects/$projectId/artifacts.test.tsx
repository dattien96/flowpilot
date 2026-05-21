import { describe, expect, it, vi } from "vitest";

import { loadProjectArtifactsData } from "./artifacts";

describe("loadProjectArtifactsData", () => {
  it("loads the project directly without requiring feature or workflow summary gateways", async () => {
    const gateways = {
      projectGateway: {
        getProjectById: vi.fn().mockResolvedValue({
          id: "project-alpha",
          name: "Alpha",
        }),
      },
      workflowEngineGateway: {
        listArtifactRuns: vi.fn().mockResolvedValue([
          {
            id: "artifact-1",
            artifactDefinitionKey: "tech_spec_artifact",
            projectId: "project-alpha",
            workflowId: "workflow-1",
            workflowRunId: "run-1",
            workflowRunStepId: "step-1",
            title: "Tech Spec",
            localPath: ".flowpilot/artifacts/project-alpha/TechSpec.md",
            remotePath: "",
            remoteUrl: "",
            syncStatus: "local_only",
            createdAt: "2026-05-21T00:00:00.000Z",
            updatedAt: "2026-05-21T00:00:00.000Z",
          },
        ]),
      },
    };

    const result = await loadProjectArtifactsData(gateways, "project-alpha");

    expect(gateways.projectGateway.getProjectById).toHaveBeenCalledWith("project-alpha");
    expect(gateways.workflowEngineGateway.listArtifactRuns).toHaveBeenCalledWith("project-alpha");
    expect(result).toMatchObject({
      projectId: "project-alpha",
      project: {
        id: "project-alpha",
      },
      artifactRuns: [
        {
          id: "artifact-1",
        },
      ],
    });
  });
});
