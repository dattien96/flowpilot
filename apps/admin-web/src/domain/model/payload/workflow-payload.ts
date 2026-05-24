import type { ApprovalStatus } from "@/domain/constant/status";
import type { AiOutput } from "@/domain/model/entity/workflow";

export interface StartWorkflowRunPayload {
  projectId: string;
  workflowDefinitionId: string;
  contextSourceIds: string[];
  beginPrompt?: string;
  startMode?: "workflow-definition" | "single-step" | "new-workflow";
  stepType?: string;
  reasoningEffort?: string | null;
}

export interface SubmitApprovalDecisionPayload {
  approvalId: string;
  decision: Extract<ApprovalStatus, "approved" | "rejected" | "changes_requested">;
  comment?: string;
}

export interface ListOutputsFilters {
  projectId?: string;
  workflowRunId?: string;
  outputType?: AiOutput["outputType"];
  approvalState?: "approved" | "pending";
}
