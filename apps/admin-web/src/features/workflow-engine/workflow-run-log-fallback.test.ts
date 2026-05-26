import { describe, expect, it } from "vitest";

import { buildFallbackOutputsFromLogs, extractBeginPromptFromLogs } from "./workflow-run-log-fallback";

describe("workflow-run-log-fallback", () => {
  it("extracts the begin prompt from workflow logs", () => {
    expect(
      extractBeginPromptFromLogs([
        {
          workflowRunStepId: "step-1",
          logLevel: "info",
          message: "Begin prompt: build a product for cafes",
          createdAt: "2026-05-26T13:27:14.000Z",
        },
      ]),
    ).toBe("build a product for cafes");
  });

  it("creates a fallback output from the latest debug log when no artifact exists", () => {
    const outputs = buildFallbackOutputsFromLogs({
      existingOutputs: [],
      logs: [
        {
          workflowRunStepId: "step-1",
          logLevel: "debug",
          message: "# Result\n\nHello from macOS.",
          createdAt: "2026-05-26T13:27:15.000Z",
        },
      ],
      projectId: "project-1",
      runId: "run-1",
      steps: [{ id: "step-1", stepName: "Test - Codex Step" }],
    });

    expect(outputs).toHaveLength(1);
    expect(outputs[0].workflowStepId).toBe("step-1");
    expect(outputs[0].contentMarkdown).toContain("Hello from macOS.");
    expect(outputs[0].title).toBe("Test - Codex Step.md");
  });

  it("prefers ai_output logs and overlays them onto existing artifact-backed outputs", () => {
    const outputs = buildFallbackOutputsFromLogs({
      existingOutputs: [
        {
          id: "artifact-1",
          workflowRunId: "run-1",
          workflowStepId: "step-1",
          projectId: "project-1",
          outputType: "document",
          version: 1,
          title: "Response.md",
          contentMarkdown: "stale artifact body",
          isApproved: true,
          createdAt: "2026-05-26T13:27:15.000Z",
          localPath: "/tmp/artifact",
        },
      ],
      logs: [
        {
          workflowRunStepId: "step-1",
          logLevel: "debug",
          message: "ai_output:# Result\n\nFresh provider output.",
          createdAt: "2026-05-26T13:27:16.000Z",
        },
      ],
      projectId: "project-1",
      runId: "run-1",
      steps: [{ id: "step-1", stepName: "Test - Codex Step" }],
    });

    expect(outputs).toHaveLength(1);
    expect(outputs[0].contentMarkdown).toContain("Fresh provider output.");
    expect(outputs[0].localPath).toBe("/tmp/artifact");
  });

  it("reuses the existing artifact record while preferring fresher log output", () => {
    const outputs = buildFallbackOutputsFromLogs({
      existingOutputs: [
        {
          id: "artifact-1",
          workflowRunId: "run-1",
          workflowStepId: "step-1",
          projectId: "project-1",
          outputType: "document",
          version: 1,
          title: "Response.md",
          contentMarkdown: "existing",
          isApproved: true,
          createdAt: "2026-05-26T13:27:15.000Z",
        },
      ],
      logs: [
        {
          workflowRunStepId: "step-1",
          logLevel: "debug",
          message: "# Result\n\nHello from macOS.",
          createdAt: "2026-05-26T13:27:15.000Z",
        },
      ],
      projectId: "project-1",
      runId: "run-1",
      steps: [{ id: "step-1", stepName: "Test - Codex Step" }],
    });

    expect(outputs).toHaveLength(1);
    expect(outputs[0].id).toBe("artifact-1");
    expect(outputs[0].contentMarkdown).toContain("Hello from macOS.");
  });

  it("only applies the latest step log to the newest output version for that step", () => {
    const outputs = buildFallbackOutputsFromLogs({
      existingOutputs: [
        {
          id: "artifact-1",
          workflowRunId: "run-1",
          workflowStepId: "step-1",
          projectId: "project-1",
          outputType: "document",
          version: 1,
          title: "Response.md",
          contentMarkdown: "first response",
          isApproved: true,
          createdAt: "2026-05-26T13:27:15.000Z",
        },
        {
          id: "artifact-2",
          workflowRunId: "run-1",
          workflowStepId: "step-1",
          projectId: "project-1",
          outputType: "document",
          version: 2,
          title: "Response.md",
          contentMarkdown: "second response",
          isApproved: true,
          createdAt: "2026-05-26T13:27:25.000Z",
        },
      ],
      logs: [
        {
          workflowRunStepId: "step-1",
          logLevel: "debug",
          message: "ai_output:latest response only",
          createdAt: "2026-05-26T13:27:26.000Z",
        },
      ],
      projectId: "project-1",
      runId: "run-1",
      steps: [{ id: "step-1", stepName: "Test - Codex Step" }],
    });

    expect(outputs).toHaveLength(2);
    expect(outputs[0].id).toBe("artifact-1");
    expect(outputs[0].contentMarkdown).toBe("first response");
    expect(outputs[1].id).toBe("artifact-2");
    expect(outputs[1].contentMarkdown).toBe("latest response only");
  });
});
