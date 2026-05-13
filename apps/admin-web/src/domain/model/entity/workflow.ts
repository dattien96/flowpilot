import type {
  ApprovalStatus,
  OutputType,
  WorkflowRunStatus,
  WorkflowStepStatus,
} from "@/domain/constant/status";

export interface WorkflowDefinitionStep {
  key: string;
  name: string;
  type: "tool" | "ai_mock" | "approval";
  outputType?: OutputType;
}

export interface WorkflowDefinition {
  id: string;
  name: string;
  description: string;
  version: number;
  status: "active" | "archived";
  steps: WorkflowDefinitionStep[];
}

export interface WorkflowRun {
  id: string;
  workflowDefinitionId: string;
  projectId: string;
  featureId: string;
  status: WorkflowRunStatus;
  currentStepKey: string | null;
  selectedContextSourceIds: string[];
  startedBy: string;
  startedAt: string;
  completedAt: string | null;
  errorSummary: string | null;
}

export interface WorkflowStep {
  id: string;
  workflowRunId: string;
  stepKey: string;
  stepName: string;
  stepType: WorkflowDefinitionStep["type"];
  status: WorkflowStepStatus;
  sequenceIndex: number;
  outputId: string | null;
  startedAt: string | null;
  completedAt: string | null;
  errorMessage: string | null;
}

export interface AiOutput {
  id: string;
  workflowRunId: string;
  workflowStepId: string;
  projectId: string;
  featureId: string;
  outputType: OutputType;
  version: number;
  title: string;
  contentMarkdown: string;
  isApproved: boolean;
  createdAt: string;
}

export interface Approval {
  id: string;
  workflowRunId: string;
  workflowStepId: string;
  aiOutputId: string | null;
  status: ApprovalStatus;
  reviewerId: string | null;
  comment: string | null;
  decidedAt: string | null;
  createdAt: string;
}

export interface AiCallLog {
  id: string;
  workflowRunId: string;
  workflowStepId: string;
  provider: string;
  model: string;
  inputTokens: number;
  outputTokens: number;
  costEstimate: number;
  latencyMs: number;
  status: "success" | "failed";
  createdAt: string;
}
