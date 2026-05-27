import { describe, expect, it } from "vitest";

import {
  buildWorkflowStepSessionGroups,
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
          actualPromptText: "Bootstrap replay prompt 1",
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
          actualPromptText: "Bootstrap replay prompt 2",
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
    expect(merged[0]?.actualPromptText).toBe("Bootstrap replay prompt 1");
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

  it("groups prompts and outputs under the session that handled them", () => {
    const groups = buildWorkflowStepSessionGroups({
      outputs: [
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
          createdAt: "2026-05-26T08:39:20.000Z",
        },
      ],
      decisions: [
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
        {
          id: "decision-2",
          approvalId: "approval-step-1",
          workflowRunId: "run-1",
          workflowStepId: "step-1",
          aiOutputId: null,
          decision: "changes_requested",
          reviewerId: null,
          comment: "retry in new session",
          createdAt: "2026-05-26T08:38:55.000Z",
        },
      ],
      sessions: [
        {
          id: "session-1",
          workflowRunId: "run-1",
          provider: "codex",
          model: "gpt-5.4-mini",
          transportType: "codex_mcp",
          providerSessionId: "thread-main",
          processKey: "proc-1",
          status: "completed",
          metadataJson: { is_main: true },
          startedAt: "2026-05-26T08:36:00.000Z",
          completedAt: "2026-05-26T08:38:59.000Z",
        },
        {
          id: "session-2",
          workflowRunId: "run-1",
          provider: "codex",
          model: "gpt-5.4-mini",
          transportType: "codex_mcp",
          providerSessionId: "thread-replay",
          processKey: "proc-2",
          status: "active",
          metadataJson: {
            is_main: true,
            recovery: { mode: "bootstrap_replay", replayCheckpointCount: 2 },
          },
          startedAt: "2026-05-26T08:39:00.000Z",
          completedAt: null,
        },
      ],
      sessionStarts: [
        {
          createdAt: "2026-05-26T08:36:00.000Z",
          providerSessionId: "thread-main",
          sessionId: "session-1",
        },
        {
          createdAt: "2026-05-26T08:39:00.000Z",
          providerSessionId: "thread-replay",
          sessionId: "session-2",
        },
      ],
      initialPrompt: {
        createdAt: "2026-05-26T08:36:30.000Z",
        content: "Give me the business idea of this project",
      },
    });

    expect(groups).toHaveLength(2);
    expect(groups[0]?.items.map((item) => item.key)).toEqual([
      "prompt-initial",
      "output-artifact-1",
      "prompt-decision-decision-1",
    ]);
    expect(groups[1]?.items.map((item) => item.key)).toEqual([
      "prompt-decision-decision-2",
      "output-artifact-2",
    ]);
    expect(groups[1]?.items[0]).toMatchObject({
      kind: "prompt",
      title: "Follow-up",
      metaNote: "Replayed 2 previous prompts into this new session before continuing.",
    });

    // Verify structured promptGroups hierarchy
    expect(groups[0]?.promptGroups).toHaveLength(2);
    expect(groups[0]?.promptGroups[0]?.prompt.key).toBe("prompt-initial");
    expect(groups[0]?.promptGroups[0]?.attempts.map((a) => a.key)).toEqual(["output-artifact-1"]);
    expect(groups[0]?.promptGroups[1]?.prompt.key).toBe("prompt-decision-decision-1");
    expect(groups[0]?.promptGroups[1]?.attempts).toHaveLength(0);

    expect(groups[1]?.promptGroups).toHaveLength(1);
    expect(groups[1]?.promptGroups[0]?.prompt.key).toBe("prompt-decision-decision-2");
    expect(groups[1]?.promptGroups[0]?.attempts.map((a) => a.key)).toEqual(["output-artifact-2"]);
  });
});
