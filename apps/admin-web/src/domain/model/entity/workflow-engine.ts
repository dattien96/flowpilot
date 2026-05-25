export type StepType = string;

export type ArtifactSyncStatus = "local_only" | "queued" | "syncing" | "synced" | "failed";

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

export const STEP_MODEL_OPTIONS = [
  { value: "gemini-flash", label: "Gemini Flash" },
  { value: "gemini-pro", label: "Gemini Pro" },
  { value: "claude-haiku", label: "Claude Haiku" },
  { value: "claude-sonnet", label: "Claude Sonnet" },
  { value: "claude-opus", label: "Claude Opus" },
  { value: "gpt-5.4-mini", label: "GPT 5.4 Mini" },
  { value: "gpt-5.4", label: "GPT 5.4" },
  { value: "gpt-5.5", label: "GPT 5.5" },
] as const;

export type SupportedStepModel = (typeof STEP_MODEL_OPTIONS)[number]["value"];

export type ReasoningEffort = "low" | "medium" | "high" | "xhigh";

export const REASONING_EFFORT_OPTIONS = [
  { value: "low", label: "Low" },
  { value: "medium", label: "Medium" },
  { value: "high", label: "High" },
  { value: "xhigh", label: "Extra High" },
] as const;

export type WorkflowStartMode = "workflow-definition" | "single-step";

export type WorkflowRunStartRequest =
  | {
      workflowId: string;
      projectId: string;
      startMode: "workflow-definition";
      beginPrompt: string;
      stepType?: null;
    }
  | {
      workflowId?: string | null;
      projectId: string;
      startMode: "single-step";
      beginPrompt: string;
      stepType: StepType;
    };

export function isSupportedStepModel(value: string): value is SupportedStepModel {
  return STEP_MODEL_OPTIONS.some((option) => option.value === value);
}

export function deriveStepPromptBase({
  description,
  name,
  stepType,
}: {
  stepType: StepType;
  name: string;
  description: string;
}) {
  const trimmedDescription = description.trim();
  if (trimmedDescription) {
    return `You are executing the "${name}" workflow step.\n\n${trimmedDescription}`;
  }

  return `You are executing the "${name}" workflow step (${stepType}). Produce the expected deliverable for this stage.`;
}

export interface StepDefinition {
  stepType: StepType;
  name: string;
  description: string;
  promptBase: string | null;
  requiredMcps: string[];
  requiredSkills: string[];
  teamRole?: string | null;
  subagent?: string | null;
  model: SupportedStepModel;
  agentType: "standard" | "autonomous";
  inputArtifactDefinitions?: string[];
  outputArtifactDefinitions?: string[];
  createdAt: string;
  updatedAt: string;
}

export interface ArtifactDefinition {
  key: string;
  name: string;
  description: string;
  localPathTemplate: string;
  remotePathTemplate: string;
  defaultFileName: string;
  createdAt: string;
  updatedAt: string;
}

export interface ArtifactRun {
  id: string;
  artifactDefinitionKey: string;
  workflowId: string;
  workflowRunId: string;
  workflowRunStepId: string | null;
  projectId: string | null;
  title: string;
  localPath: string;
  remotePath: string;
  remoteUrl: string;
  syncStatus: ArtifactSyncStatus;
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
  reasoningEffortOverride?: string | null;
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
  reasoningEffortOverride?: string | null;
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
  reasoningEffort?: string | null;
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
  artifactRunId?: string | null;
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
