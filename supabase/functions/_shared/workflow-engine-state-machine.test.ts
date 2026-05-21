import { describe, expect, it } from "vitest";

import {
  planRejectedStepRetry,
  planWorkflowProgress,
  type RuntimeWorkflowStep,
} from "./workflow-engine-state-machine";

describe("workflow-engine-state-machine", () => {
  it("pauses on the first approval gate when YOLO is disabled", () => {
    const plan = planWorkflowProgress({
      yoloMode: false,
      now: "2026-05-20T01:00:00Z",
      steps: [
        {
          id: "step-1",
          stepType: "tech_spec",
          status: "PENDING",
          requiresApproval: true,
          startedAt: null,
          retryCount: 0,
          rejectionNote: null,
        },
      ],
    });

    expect(plan.runStatus).toBe("RUNNING");
    expect(plan.pausedStepId).toBe("step-1");
    expect(plan.stepTransitions).toEqual([
      {
        stepId: "step-1",
        patch: {
          status: "WAITING_USER_APPROVAL",
          startedAt: "2026-05-20T01:00:00Z",
          finishedAt: null,
        },
        logs: [
          {
            logLevel: "info",
            message: "Generation complete. Paused on approval gate for tech_spec.",
          },
        ],
      },
    ]);
  });

  it("continues through remaining steps when YOLO is enabled", () => {
    const steps: RuntimeWorkflowStep[] = [
      {
        id: "step-1",
        stepType: "tech_spec",
        status: "WAITING_USER_APPROVAL",
        requiresApproval: true,
        startedAt: "2026-05-20T00:58:00Z",
        retryCount: 0,
        rejectionNote: null,
      },
      {
        id: "step-2",
        stepType: "telegram_notification",
        status: "PENDING",
        requiresApproval: false,
        startedAt: null,
        retryCount: 0,
        rejectionNote: null,
      },
    ];

    const plan = planWorkflowProgress({
      yoloMode: true,
      now: "2026-05-20T01:00:00Z",
      steps,
    });

    expect(plan.runStatus).toBe("DONE");
    expect(plan.pausedStepId).toBe(null);
    expect(plan.stepTransitions).toEqual([
      {
        stepId: "step-1",
        patch: {
          status: "DONE",
          startedAt: "2026-05-20T00:58:00Z",
          finishedAt: "2026-05-20T01:00:00Z",
          rejectionNote: null,
        },
        logs: [
          {
            logLevel: "info",
            message: "YOLO mode auto-approved step tech_spec.",
          },
        ],
      },
      {
        stepId: "step-2",
        patch: {
          status: "DONE",
          startedAt: "2026-05-20T01:00:00Z",
          finishedAt: "2026-05-20T01:00:00Z",
          rejectionNote: null,
        },
        logs: [
          {
            logLevel: "info",
            message: "Completed step telegram_notification.",
          },
        ],
      },
    ]);
  });

  it("prepares a rejected step for another pass", () => {
    const plan = planRejectedStepRetry({
      stepId: "step-3",
      now: "2026-05-20T01:00:00Z",
      comment: "Tighten the spec before coding.",
      step: {
        status: "WAITING_USER_APPROVAL",
        retryCount: 2,
      },
    });

    expect(plan).toEqual({
      stepId: "step-3",
      patch: {
        status: "PENDING",
        startedAt: null,
        finishedAt: null,
        rejectionNote: "Tighten the spec before coding.",
        retryCount: 3,
      },
      logs: [
        {
          logLevel: "warn",
          message:
            'Rejection received at 2026-05-20T01:00:00Z: "Tighten the spec before coding.". Resetting step for another pass.',
        },
      ],
    });
  });
});
