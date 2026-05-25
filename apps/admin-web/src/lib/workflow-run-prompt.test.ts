import { describe, expect, it, vi } from "vitest";

import { loadWorkflowRunPromptText } from "./workflow-run-prompt";

describe("workflow-run-prompt", () => {
  it("reads the prompt from workflow_output when it is available", async () => {
    const gateway = {
      listArtifacts: vi.fn(async () => [
        {
          artifactId: "workflow-output-1",
          title: "BusinessIdea.md",
          sourceKind: "workflow_output",
          projectId: "project-1",
          workflowRunId: "0cd0fe76-87a1-497d-af5e-b773a386de67",
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
      ]),
      getArtifactById: vi.fn(async (artifactId: string) => {
        if (artifactId === "workflow-output-1") {
          return {
            artifactId,
            promptText: `## Begin Prompt
build a product for cafes
            `,
          };
        }

        return null;
      }),
    };

    await expect(
      loadWorkflowRunPromptText(
        gateway,
        "0cd0fe76-87a1-497d-af5e-b773a386de67",
      ),
    ).resolves.toBe("## Begin Prompt\nbuild a product for cafes");
  });

  it("falls back to prompt_execution artifacts when workflow_output is empty", async () => {
    const workflowRunId = "0cd0fe76-87a1-497d-af5e-b773a386de67";
    const gateway = {
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

    await expect(
      loadWorkflowRunPromptText(gateway, workflowRunId),
    ).resolves.toContain("give me answear to know you alive");
  });
});
