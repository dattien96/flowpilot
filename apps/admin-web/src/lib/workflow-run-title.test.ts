import { describe, expect, it, vi } from "vitest";

import { loadWorkflowRunTitleMap, summarizeRunPrompt } from "./workflow-run-title";

describe("workflow-run-title", () => {
  it("extracts the user prompt from the Begin Prompt section", () => {
    expect(
      summarizeRunPrompt(`
## Prompt Base
You are executing a step.

## Begin Prompt
build a product for cafes

## Input Artifacts
- None
      `),
    ).toBe("build a product for cafes");
  });

  it("falls back to prompt_execution artifacts when workflow_output has no prompt text", async () => {
    const workflowRunId = "0cd0fe76-87a1-497d-af5e-b773a386de67";
    const localRunnerGateway = {
      listArtifacts: vi.fn(async () => [
        {
          artifactId: "workflow-output-1",
          title: "BusinessIdea.md",
          sourceKind: "workflow_output",
          projectId: "project-1",
          workflowRunId,
          workflowStepKey: "business_idea",
          providerKey: "codex",
          localPath: ".flowpilot/artifacts/project-1/0cd0fe76-87a1-497d-af5e-b773a386de67/business_idea",
          remotePath: "",
          remoteUrl: "",
          syncStatus: "local_only" as const,
          createdAt: "2026-05-23T01:47:10.000Z",
          updatedAt: "2026-05-23T01:47:24.000Z",
          contentMarkdown: "",
          previewMarkdown: "",
        },
        {
          artifactId: "prompt-execution-1",
          title: "Prompt Execution Result",
          sourceKind: "prompt_execution",
          projectId: "local",
          workflowRunId: "prompt_20260523_014710_8000",
          workflowStepKey: "prompt_execution",
          providerKey: "codex",
          localPath:
            "/Users/tiendat/Desktop/flowpilot/flowpilot/.flowpilot/artifacts/local/prompt-execution/prompt_20260523_014710_8000/prompt_execution/artifact_prompt_20260523_014710_8000",
          remotePath: "",
          remoteUrl: "",
          syncStatus: "local_only" as const,
          createdAt: "2026-05-23T01:47:10.176Z",
          updatedAt: "2026-05-23T01:47:24.785Z",
          contentMarkdown: "",
          previewMarkdown: "",
        },
      ]),
      getArtifactById: vi.fn(async (artifactId: string) => {
        if (artifactId === "workflow-output-1") {
          return {
            artifactId,
            promptText: "",
          };
        }
        if (artifactId === "prompt-execution-1") {
          return {
            artifactId,
            promptText: `# Workflow Step Execution

## Begin Prompt
give me answear to know you alive

## Output Targets
- /Users/tiendat/Desktop/flowpilot/flowpilot/.flowpilot/artifacts/392355a6-5573-44f1-9aa5-4313502a5816/${workflowRunId}/business_idea/BusinessIdea.md
            `,
          };
        }
        return null;
      }),
    };

    const titleMap = await loadWorkflowRunTitleMap(localRunnerGateway, [
      workflowRunId,
    ]);

    expect(titleMap.get(workflowRunId)).toBe("give me answear to know you alive");
  });

  it("resolves run ids from Windows prompt_execution paths", async () => {
    const workflowRunId = "0cd0fe76-87a1-497d-af5e-b773a386de67";
    const localRunnerGateway = {
      listArtifacts: vi.fn(async () => [
        {
          artifactId: "prompt-execution-1",
          title: "Prompt Execution Result",
          sourceKind: "prompt_execution",
          projectId: "local",
          workflowRunId: "prompt_20260523_014710_8000",
          workflowStepKey: "prompt_execution",
          providerKey: "codex",
          localPath:
            "C:\\working\\flowpilot\\.flowpilot\\artifacts\\local\\prompt-execution\\prompt_20260523_014710_8000\\prompt_execution\\artifact_prompt_20260523_014710_8000",
          remotePath: "",
          remoteUrl: "",
          syncStatus: "local_only" as const,
          createdAt: "2026-05-23T01:47:10.176Z",
          updatedAt: "2026-05-23T01:47:24.785Z",
          contentMarkdown: "",
          previewMarkdown: "",
        },
      ]),
      getArtifactById: vi.fn(async () => ({
        artifactId: "prompt-execution-1",
        promptText: `# Workflow Step Execution

## Begin Prompt
build a product for cafes

## Output Targets
- C:\\working\\flowpilot\\.flowpilot\\artifacts\\392355a6-5573-44f1-9aa5-4313502a5816\\${workflowRunId}\\business_idea\\BusinessIdea.md`,
      })),
    };

    const titleMap = await loadWorkflowRunTitleMap(localRunnerGateway, [
      workflowRunId,
    ]);

    expect(titleMap.get(workflowRunId)).toBe("build a product for cafes");
  });

  it("falls back to synced artifact run titles when local artifacts are gone", async () => {
    const workflowRunId = "0cd0fe76-87a1-497d-af5e-b773a386de67";
    const localRunnerGateway = {
      listArtifacts: vi.fn(async () => []),
      getArtifactById: vi.fn(async () => null),
    };
    const workflowEngineGateway = {
      listArtifactRuns: vi.fn(async () => [
        {
          id: "artifact-run-1",
          artifactDefinitionKey: "business_idea_artifact",
          workflowId: "workflow-1",
          workflowRunId,
          workflowRunStepId: "step-1",
          projectId: "project-1",
          title: "BusinessIdea.md",
          localPath: ".flowpilot/artifacts/project-1/run-1/business_idea/.snapshots/artifact-run-1/BusinessIdea.md",
          remotePath: "projects/project-1/runs/run-1/steps/business_idea/BusinessIdea.md",
          remoteUrl: "",
          storageProvider: "supabase" as const,
          remoteObjectId: "object-1",
          syncStatus: "synced" as const,
          createdAt: "2026-05-23T01:47:10.176Z",
          updatedAt: "2026-05-23T01:47:24.785Z",
        },
      ]),
    };

    const titleMap = await loadWorkflowRunTitleMap(
      localRunnerGateway,
      [workflowRunId],
      {
        listArtifactRuns: workflowEngineGateway.listArtifactRuns,
      },
    );

    expect(titleMap.get(workflowRunId)).toBe("BusinessIdea.md");
  });

  it("uses a 50 character remote content summary for generic synced artifact titles", async () => {
    const workflowRunId = "0cd0fe76-87a1-497d-af5e-b773a386de67";
    const localRunnerGateway = {
      listArtifacts: vi.fn(async () => []),
      getArtifactById: vi.fn(async () => null),
    };
    const artifactRun = {
      id: "artifact-run-1",
      artifactDefinitionKey: null,
      workflowId: "workflow-1",
      workflowRunId,
      workflowRunStepId: "step-1",
      projectId: "project-1",
      title: "Response.md",
      localPath: "",
      remotePath: "projects/project-1/runs/run-1/steps/business_idea/Response.md",
      remoteUrl: "",
      storageProvider: "supabase" as const,
      remoteObjectId: "object-1",
      syncStatus: "synced" as const,
      createdAt: "2026-05-23T01:47:10.176Z",
      updatedAt: "2026-05-23T01:47:24.785Z",
    };
    const loadArtifactContent = vi.fn(
      async () => "This is a synced remote response title that should be shortened for cards.",
    );

    const titleMap = await loadWorkflowRunTitleMap(
      localRunnerGateway,
      [workflowRunId],
      {
        listArtifactRuns: vi.fn(async () => [artifactRun]),
        loadArtifactContent,
      },
    );

    expect(titleMap.get(workflowRunId)).toBe("This is a synced remote response title that should...");
    expect(loadArtifactContent).toHaveBeenCalledWith(artifactRun);
  });
});
