import type { ApprovalStatus } from "@/domain/constant/status";

export interface StartWorkflowRunPayload {
  featureId: string;
  workflowDefinitionId: string;
  contextSourceIds: string[];
}

export interface SubmitApprovalDecisionPayload {
  approvalId: string;
  decision: Extract<ApprovalStatus, "approved" | "rejected" | "changes_requested">;
  comment?: string;
}
