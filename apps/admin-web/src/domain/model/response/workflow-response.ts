import type {
  AiCallLog,
  AiOutput,
  Approval,
  WorkflowRun,
  WorkflowStep,
} from "@/domain/model/entity/workflow";

export interface WorkflowRunDetail {
  run: WorkflowRun;
  steps: WorkflowStep[];
  approvals: Approval[];
  outputs: AiOutput[];
  logs: AiCallLog[];
}
