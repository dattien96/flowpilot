export type StepType = string;

export type WorkflowStepStatus =
  | "PENDING"
  | "RUNNING"
  | "WAITING_USER_APPROVAL"
  | "DONE"
  | "FAILED"
  | "SKIPPED";

export type WorkflowRunStatus =
  | "PENDING"
  | "RUNNING"
  | "DONE"
  | "FAILED"
  | "CANCELED";

export interface StepDefinition {
  stepType: StepType;
  name: string;
  description: string;
  requiredMcps: string[];
  requiredSkills: string[];
  agentType: "standard" | "autonomous";
  createdAt: string;
  updatedAt: string;
}

export interface Workflow {
  id: string;
  projectId: string | null;
  name: string;
  description: string;
  isTemplate: boolean;
  providerOverride: string | null;
  modelOverride: string | null;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
  steps?: WorkflowStep[];
}

export interface WorkflowStep {
  id: string;
  workflowId: string;
  stepType: StepType;
  orderIndex: number;
  isEnabled: boolean;
  providerOverride: string | null;
  modelOverride: string | null;
  requiresApproval: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface WorkflowPromptCache {
  id: string;
  workflowId: string;
  configHash: string;
  filePath: string;
  stepType: StepType;
  provider: "claude" | "codex" | "gemini";
  isValid: boolean;
  createdAt: string;
  invalidatedAt: string | null;
}

export interface WorkflowRun {
  id: string;
  workflowId: string;
  projectId: string;
  status: WorkflowRunStatus;
  provider: string | null;
  model: string | null;
  yoloMode: boolean;
  startedBy: string;
  startedAt: string;
  finishedAt: string | null;
  errorMessage: string | null;
}

export interface WorkflowRunStep {
  id: string;
  workflowRunId: string;
  workflowStepId: string | null;
  executionOrderIndex: number;
  stepType: StepType;
  status: WorkflowStepStatus;
  artifactId: string | null;
  promptCacheId: string | null;
  rejectionNote: string | null;
  retryCount: number;
  startedAt: string | null;
  finishedAt: string | null;
  errorMessage: string | null;
}

export interface WorkflowRunLog {
  id: string;
  workflowRunStepId: string;
  logLevel: "info" | "warn" | "error" | "debug";
  message: string;
  createdAt: string;
}

export interface WorkflowTemplate {
  id: string;
  name: string;
  description: string;
  steps: WorkflowStep[];
}

export interface PromptCacheEntry {
  id: string;
  workflowId: string;
  configHash: string;
  filePath: string;
  stepType: StepType;
  provider: "claude" | "codex" | "gemini";
  isValid: boolean;
  createdAt: string;
  invalidatedAt: string | null;
}
