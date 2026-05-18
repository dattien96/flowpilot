import { describe, expect, it } from "vitest";

import type { ApprovalStatus, OutputType, WorkflowRunStatus, WorkflowStepStatus } from "./status";

describe("status constants", () => {
  it("exports the workflow and approval status values required by the migrated app", () => {
    const workflowRunStatuses: WorkflowRunStatus[] = [
      "draft",
      "running",
      "waiting_approval",
      "completed",
      "rejected",
      "failed",
    ];
    const workflowStepStatuses: WorkflowStepStatus[] = [
      "pending",
      "running",
      "waiting_approval",
      "completed",
      "rejected",
      "failed",
    ];
    const approvalStatuses: ApprovalStatus[] = [
      "pending",
      "approved",
      "rejected",
      "changes_requested",
    ];
    const outputTypes: OutputType[] = [
      "business_summary",
      "product_spec",
      "android_tech_spec",
      "task_breakdown",
      "test_plan",
      "risk_report",
    ];

    expect(workflowRunStatuses).toContain("completed");
    expect(workflowStepStatuses).toContain("waiting_approval");
    expect(approvalStatuses).toContain("changes_requested");
    expect(outputTypes).toContain("task_breakdown");
  });
});
