import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { loadProjectArtifactsData } from "./artifacts";

vi.mock("@/data/repository/browser-factory", () => ({
  createGatewayBundle: vi.fn(),
}));

vi.mock("@/components/common/page-frame", () => ({
  PageFrame: () => <div />,
}));

vi.mock("@/components/project/project-section-nav", () => ({
  ProjectSectionNav: () => <div />,
}));

vi.mock("@/components/ui/button", () => ({
  Button: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("@/presentation/components/artifacts/artifact-run-browser-panel", () => ({
  ArtifactRunBrowserPanel: () => <div />,
}));

describe("loadProjectArtifactsData", () => {
  it("loads valid local artifacts and remote runs for the requested project", async () => {
    const gateways = {
      projectGateway: {
        getProjectById: vi.fn().mockResolvedValue({
          id: "project-alpha",
          name: "Alpha",
        }),
      },
      localRunnerGateway: {
        listArtifacts: vi.fn().mockResolvedValue([
          {
            artifactId: "artifact-local-1",
            title: "Local Draft",
            sourceKind: "workflow_output",
            projectId: "project-alpha",
            workflowRunId: "run-1",
            workflowStepKey: "step-1",
            providerKey: "gemini",
            localPath: ".flowpilot/artifacts/project-alpha/run-1/step-1.md",
            remotePath: "",
            remoteUrl: "",
            storageProvider: null,
            remoteObjectId: null,
            syncStatus: "local_only",
            createdAt: "2026-05-21T00:00:00.000Z",
            updatedAt: "2026-05-21T00:00:00.000Z",
            contentMarkdown: "local",
            previewMarkdown: "local",
          },
          {
            artifactId: "artifact-orphan",
            title: "Orphan Draft",
            sourceKind: "workflow_output",
            projectId: "project-alpha",
            workflowRunId: "run-missing",
            workflowStepKey: "step-x",
            providerKey: "gemini",
            localPath: ".flowpilot/artifacts/project-alpha/run-missing/step-x.md",
            remotePath: "",
            remoteUrl: "",
            storageProvider: null,
            remoteObjectId: null,
            syncStatus: "local_only",
            createdAt: "2026-05-21T00:00:00.000Z",
            updatedAt: "2026-05-21T00:00:00.000Z",
            contentMarkdown: "orphan",
            previewMarkdown: "orphan",
          },
        ]),
      },
      workflowEngineGateway: {
        listArtifactRuns: vi.fn().mockResolvedValue([
          {
            id: "artifact-remote-1",
            artifactDefinitionKey: "tech-spec",
            workflowId: "workflow-1",
            workflowRunId: "run-1",
            workflowRunStepId: "step-1",
            projectId: "project-alpha",
            title: "Tech Spec",
            localPath: ".flowpilot/artifacts/project-alpha/run-1/step-1.md",
            remotePath: "projects/project-alpha/runs/run-1/steps/step-1/TechSpec.md",
            remoteUrl: "https://example.test/object-1",
            storageProvider: "supabase",
            remoteObjectId: "object-1",
            syncStatus: "synced",
            createdAt: "2026-05-21T00:00:00.000Z",
            updatedAt: "2026-05-21T00:05:00.000Z",
          },
        ]),
        listWorkflowRuns: vi.fn().mockResolvedValue([
          {
            id: "run-1",
            workflowId: "workflow-1",
            projectId: "project-alpha",
            status: "DONE",
            provider: null,
            model: null,
            reasoningEffort: null,
            yoloMode: false,
            startedBy: "user@example.com",
            startedAt: "2026-05-21T00:00:00.000Z",
            finishedAt: "2026-05-21T00:05:00.000Z",
            errorMessage: null,
          },
        ]),
      },
    };

    const result = await loadProjectArtifactsData(gateways, "project-alpha");

    expect(gateways.projectGateway.getProjectById).toHaveBeenCalledWith("project-alpha");
    expect(gateways.localRunnerGateway.listArtifacts).toHaveBeenCalled();
    expect(gateways.workflowEngineGateway.listArtifactRuns).toHaveBeenCalledWith("project-alpha");
    expect(gateways.workflowEngineGateway.listWorkflowRuns).toHaveBeenCalledWith("project-alpha");
    expect(result).toMatchObject({
      projectId: "project-alpha",
      project: {
        id: "project-alpha",
      },
      localArtifacts: [
        {
          artifactId: "artifact-local-1",
        },
      ],
      artifactRuns: [
        {
          id: "artifact-remote-1",
        },
      ],
    });
  });
});
