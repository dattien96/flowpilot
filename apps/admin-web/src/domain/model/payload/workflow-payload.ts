import type { ApprovalStatus } from "@/domain/constant/status";
import type { AiOutput } from "@/domain/model/entity/workflow";

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

export interface ListOutputsFilters {
  projectId?: string;
  featureId?: string;
  workflowRunId?: string;
  outputType?: AiOutput["outputType"];
  approvalState?: "approved" | "pending";
}
