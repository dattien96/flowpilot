import { describe, expect, it } from "vitest";

import { applyOptimisticWorkflowFollowUp } from "./workflow-run-detail-optimistic";

describe("applyOptimisticWorkflowFollowUp", () => {
  it("marks the targeted step and run as running and appends the follow-up decision", () => {
    const detail = {
      run: {
        id: "run-1",
        workflowDefinitionId: "workflow-1",
        projectId: "project-1",
        status: "completed" as const,
        currentStepKey: null,
        selectedContextSourceIds: [],
        startedBy: "admin",
        startedAt: "2026-05-26T10:00:00.000Z",
        completedAt: "2026-05-26T10:05:00.000Z",
        errorSummary: "old error",
      },
      steps: [
        {
          id: "step-1",
          workflowRunId: "run-1",
          stepKey: "plan",
          stepName: "Plan",
          stepType: "ai_mock" as const,
          status: "completed" as const,
          sequenceIndex: 0,
          outputId: "output-1",
          startedAt: "2026-05-26T10:00:00.000Z",
          completedAt: "2026-05-26T10:03:00.000Z",
          errorMessage: "old error",
        },
      ],
      approvals: [
        {
          id: "approval_1",
          workflowRunId: "run-1",
          workflowStepId: "step-1",
          aiOutputId: "output-1",
          status: "pending" as const,
          reviewerId: null,
          comment: null,
          decidedAt: null,
          createdAt: "2026-05-26T10:03:00.000Z",
        },
      ],
      outputs: [],
      logs: [],
      approvalDecisions: [],
      selectedContextSources: [],
      project: null,
      definition: null,
    };

    const updated = applyOptimisticWorkflowFollowUp(
      detail,
      "step-1",
      "Tighten the scope.",
      "2026-05-26T10:06:00.000Z",
    );

    expect(updated.run.status).toBe("running");
    expect(updated.run.currentStepKey).toBe("plan");
    expect(updated.run.completedAt).toBeNull();
    expect(updated.run.errorSummary).toBeNull();
    expect(updated.steps[0]?.status).toBe("running");
    expect(updated.steps[0]?.completedAt).toBeNull();
    expect(updated.steps[0]?.errorMessage).toBeNull();
    expect(updated.approvals).toHaveLength(0);
    expect(updated.approvalDecisions).toEqual([
      expect.objectContaining({
        workflowStepId: "step-1",
        decision: "changes_requested",
        comment: "Tighten the scope.",
      }),
    ]);
  });
});
