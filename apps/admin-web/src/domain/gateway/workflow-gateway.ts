import type {
  AiCallLog,
  AiOutput,
  Approval,
  ApprovalDecision,
  WorkflowDefinition,
  WorkflowRun,
  WorkflowStep,
} from "@/domain/model/entity/workflow";
import type { StartWorkflowRunPayload } from "@/domain/model/payload/workflow-payload";
import type { ListOutputsFilters } from "@/domain/model/payload/workflow-payload";
import type {
  AiLogSummary,
  AiOutputDetail,
  PendingApprovalDetail,
  WorkflowRunDetail,
} from "@/domain/model/response/workflow-response";

export interface WorkflowGateway {
  listWorkflowDefinitions(): Promise<WorkflowDefinition[]>;
  getWorkflowDefinitionById(workflowDefinitionId: string): Promise<WorkflowDefinition | null>;
  listWorkflowRuns(): Promise<WorkflowRun[]>;
  getWorkflowRunById(runId: string): Promise<WorkflowRun | null>;
  getWorkflowRunDetail(runId: string): Promise<WorkflowRunDetail | null>;
  deleteWorkflowRuns(runIds: string[]): Promise<void>;
  createWorkflowRun(payload: StartWorkflowRunPayload, definition: WorkflowDefinition): Promise<WorkflowRun>;
  updateWorkflowRun(runId: string, patch: Partial<WorkflowRun>): Promise<WorkflowRun>;
  updateWorkflowStep(stepId: string, patch: Partial<WorkflowStep>): Promise<WorkflowStep>;
  createOutput(output: AiOutput): Promise<AiOutput>;
  createApproval(approval: Approval): Promise<Approval>;
  getApprovalById(approvalId: string): Promise<Approval | null>;
  listPendingApprovalDetails(): Promise<PendingApprovalDetail[]>;
  updateApproval(approvalId: string, patch: Partial<Approval>): Promise<Approval>;
  createApprovalDecision(decision: ApprovalDecision): Promise<ApprovalDecision>;
  listApprovalDecisionsByRun(runId: string): Promise<ApprovalDecision[]>;
  listOutputs(filters?: ListOutputsFilters): Promise<AiOutput[]>;
  getOutputDetail(outputId: string): Promise<AiOutputDetail | null>;
  createLog(log: AiCallLog): Promise<AiCallLog>;
  listLogs(filters?: { status?: AiCallLog["status"]; provider?: string }): Promise<AiCallLog[]>;
  getLogSummary(filters?: { status?: AiCallLog["status"]; provider?: string }): Promise<AiLogSummary>;
}

export interface WorkflowExecutorGateway {
  executeUntilPause(runId: string): Promise<void>;
}
