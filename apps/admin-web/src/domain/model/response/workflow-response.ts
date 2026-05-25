import type {
  AiCallLog,
  AiOutput,
  Approval,
  ApprovalDecision,
  WorkflowDefinition,
  WorkflowRun,
  WorkflowStep,
} from "@/domain/model/entity/workflow";
import type { ContextSource } from "@/domain/model/entity/context-source";
import type { Project } from "@/domain/model/entity/project";

export interface WorkflowRunDetail {
  run: WorkflowRun;
  steps: WorkflowStep[];
  approvals: Approval[];
  outputs: AiOutput[];
  logs: AiCallLog[];
  approvalDecisions?: ApprovalDecision[];
  selectedContextSources?: ContextSource[];
  project?: Project | null;
  definition?: WorkflowDefinition | null;
}

export interface PendingApprovalDetail {
  approval: Approval;
  run: WorkflowRun;
  step: WorkflowStep | null;
  output: AiOutput | null;
  project: Project | null;
}

export interface AiOutputDetail {
  output: AiOutput;
  run: WorkflowRun | null;
  step: WorkflowStep | null;
  project: Project | null;
  approvals: Approval[];
  approvalDecisions: ApprovalDecision[];
  versions: AiOutput[];
}

export interface AiLogSummary {
  totalCalls: number;
  totalInputTokens: number;
  totalOutputTokens: number;
  totalCostEstimate: number;
  averageLatencyMs: number;
  failedCalls: number;
}
