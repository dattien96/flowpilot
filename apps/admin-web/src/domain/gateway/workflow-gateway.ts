import type {
  AiCallLog,
  AiOutput,
  Approval,
  WorkflowDefinition,
  WorkflowRun,
  WorkflowStep,
} from "@/domain/model/entity/workflow";
import type { StartWorkflowRunPayload } from "@/domain/model/payload/workflow-payload";

export interface WorkflowGateway {
  listWorkflowDefinitions(): Promise<WorkflowDefinition[]>;
  getWorkflowDefinitionById(workflowDefinitionId: string): Promise<WorkflowDefinition | null>;
  listWorkflowRuns(): Promise<WorkflowRun[]>;
  getWorkflowRunById(runId: string): Promise<WorkflowRun | null>;
  getWorkflowRunDetail(runId: string): Promise<{
    run: WorkflowRun;
    steps: WorkflowStep[];
    outputs: AiOutput[];
    approvals: Approval[];
    logs: AiCallLog[];
  } | null>;
  createWorkflowRun(payload: StartWorkflowRunPayload, definition: WorkflowDefinition): Promise<WorkflowRun>;
  updateWorkflowRun(runId: string, patch: Partial<WorkflowRun>): Promise<WorkflowRun>;
  updateWorkflowStep(stepId: string, patch: Partial<WorkflowStep>): Promise<WorkflowStep>;
  createOutput(output: AiOutput): Promise<AiOutput>;
  createApproval(approval: Approval): Promise<Approval>;
  updateApproval(approvalId: string, patch: Partial<Approval>): Promise<Approval>;
  createLog(log: AiCallLog): Promise<AiCallLog>;
}

export interface WorkflowExecutorGateway {
  executeUntilPause(runId: string): Promise<void>;
}
