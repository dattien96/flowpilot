import { describe, expect, it } from "vitest";

import {
  canCancelWorkflowRun,
  canResumeWorkflowRun,
  getInterruptedWorkflowRunStepIds,
  INTERRUPTED_RUN_ERROR,
  isInterruptedWorkflowStep,
} from "./workflow-run-interruption";

describe("workflow-run-interruption", () => {
  it("allows resume only when pending workflow work exists", () => {
    expect(
      canResumeWorkflowRun({
        run: { status: "RUNNING" },
        steps: [{ id: "step-1", status: "PENDING" }],
      }),
    ).toBe(true);

    expect(
      canResumeWorkflowRun({
        run: { status: "RUNNING" },
        steps: [{ id: "step-1", status: "RUNNING" }],
      }),
    ).toBe(false);
  });

  it("marks running steps as interrupted when the run is still running but all sessions are gone", () => {
    const stepIds = getInterruptedWorkflowRunStepIds({
      run: { status: "RUNNING" },
      steps: [
        { id: "step-1", status: "RUNNING" },
        { id: "step-2", status: "PENDING" },
      ],
      sessions: [{ status: "completed", processKey: null }],
    });

    expect(stepIds).toEqual(["step-1"]);
  });

  it("does not mark interrupted steps before any session row exists", () => {
    const stepIds = getInterruptedWorkflowRunStepIds({
      run: { status: "RUNNING" },
      steps: [{ id: "step-1", status: "RUNNING" }],
      sessions: [],
    });

    expect(stepIds).toEqual([]);
  });

  it("does not mark interrupted steps while a live active session still exists", () => {
    const stepIds = getInterruptedWorkflowRunStepIds({
      run: { status: "RUNNING" },
      steps: [{ id: "step-1", status: "RUNNING" }],
      sessions: [{ status: "active", processKey: "proc-1" }],
    });

    expect(stepIds).toEqual([]);
  });

  it("does not allow cancel for failed runs", () => {
    expect(canCancelWorkflowRun({ status: "FAILED" })).toBe(false);
    expect(canCancelWorkflowRun({ status: "RUNNING" })).toBe(true);
  });

  it("detects interrupted failed workflow steps", () => {
    expect(
      isInterruptedWorkflowStep({
        status: "FAILED",
        errorMessage: INTERRUPTED_RUN_ERROR,
      }),
    ).toBe(true);
  });
});
