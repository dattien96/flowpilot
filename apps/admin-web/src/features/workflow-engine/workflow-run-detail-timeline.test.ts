import { describe, expect, it } from "vitest";

import {
  buildWorkflowStepTimeline,
  groupOutputsByStep,
  mapArtifactsToWorkflowOutputs,
  mergeWorkflowOutputs,
  type WorkflowOutputRecord,
} from "./workflow-run-detail-timeline";

describe("workflow-run-detail-timeline", () => {
  it("preserves multiple output versions for the same workflow step", () => {
    const baseOutputs: WorkflowOutputRecord[] = [
      {
        id: "artifact-2",
        workflowRunId: "run-1",
        workflowStepId: "step-1",
        projectId: "project-1",
        outputType: "document",
        version: 1,
        title: "BusinessIdea.md",
        contentMarkdown: "",
        isApproved: false,
        createdAt: "2026-05-26T08:38:00.000Z",
      },
    ];

    const localOutputs = mapArtifactsToWorkflowOutputs(
      [
        {
          artifactId: "artifact-1",
          title: "BusinessIdea.md",
          sourceKind: "workflow_output",
          projectId: "project-1",
          workflowRunId: "run-1",
          workflowStepKey: "business_idea",
          providerKey: "codex",
          localPath: ".flowpilot/artifacts/project-1/run-1/business_idea",
          remotePath: "",
          remoteUrl: "",
          syncStatus: "local_only",
          createdAt: "2026-05-26T08:37:40.000Z",
          updatedAt: "2026-05-26T08:37:40.000Z",
          contentMarkdown: "first output",
          previewMarkdown: "first output",
        },
        {
          artifactId: "artifact-2",
          title: "BusinessIdea.md",
          sourceKind: "workflow_output",
          projectId: "project-1",
          workflowRunId: "run-1",
          workflowStepKey: "business_idea",
          providerKey: "codex",
          localPath: ".flowpilot/artifacts/project-1/run-1/business_idea",
          remotePath: "",
          remoteUrl: "",
          syncStatus: "local_only",
          createdAt: "2026-05-26T08:38:00.000Z",
          updatedAt: "2026-05-26T08:38:00.000Z",
          contentMarkdown: "second output",
          previewMarkdown: "second output",
        },
      ],
      [
        {
          id: "step-1",
          workflowRunId: "run-1",
          stepKey: "business_idea",
          stepName: "Business Idea",
          stepType: "ai_mock",
          status: "completed",
          sequenceIndex: 0,
          outputId: null,
          startedAt: null,
          completedAt: null,
          errorMessage: null,
        },
      ],
    );

    const merged = mergeWorkflowOutputs(baseOutputs, localOutputs);

    expect(merged).toHaveLength(2);
    expect(merged.map((output) => output.id)).toEqual([
      "artifact-1",
      "artifact-2",
    ]);
    expect(merged[1]?.contentMarkdown).toBe("second output");
  });

  it("orders outputs and follow-up prompts chronologically for a step timeline", () => {
    const outputsByStep = groupOutputsByStep([
      {
        id: "artifact-1",
        workflowRunId: "run-1",
        workflowStepId: "step-1",
        projectId: "project-1",
        outputType: "document",
        version: 1,
        title: "BusinessIdea.md",
        contentMarkdown: "first output",
        isApproved: false,
        createdAt: "2026-05-26T08:37:40.000Z",
      },
      {
        id: "artifact-2",
        workflowRunId: "run-1",
        workflowStepId: "step-1",
        projectId: "project-1",
        outputType: "document",
        version: 1,
        title: "BusinessIdea.md",
        contentMarkdown: "second output",
        isApproved: false,
        createdAt: "2026-05-26T08:38:20.000Z",
      },
    ]);

    const items = buildWorkflowStepTimeline(outputsByStep.get("step-1") ?? [], [
      {
        id: "decision-1",
        approvalId: "approval-step-1",
        workflowRunId: "run-1",
        workflowStepId: "step-1",
        aiOutputId: null,
        decision: "changes_requested",
        reviewerId: null,
        comment: "hello x2",
        createdAt: "2026-05-26T08:38:00.000Z",
      },
    ]);

    expect(items.map((item) => `${item.kind}:${item.createdAt}`)).toEqual([
      "output:2026-05-26T08:37:40.000Z",
      "decision:2026-05-26T08:38:00.000Z",
      "output:2026-05-26T08:38:20.000Z",
    ]);
  });
});
