import type { WorkflowRun } from "@/domain/model/entity/workflow";

export interface DashboardSummary {
  activeWorkflowCount: number;
  pendingApprovalCount: number;
  completedOutputCount: number;
  projectCount: number;
  recentRuns: WorkflowRun[];
}
