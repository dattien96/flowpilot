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
  { value: "auto-gemini-3", label: "Auto (Gemini 3)" },
  { value: "auto-gemini-2.5", label: "Auto (Gemini 2.5)" },
  { value: "gemini-3.1-pro-preview", label: "Gemini 3.1 Pro Preview" },
  { value: "gemini-3-flash-preview", label: "Gemini 3 Flash Preview" },
  { value: "gemini-3.1-flash-lite-preview", label: "Gemini 3.1 Flash Lite Preview" },
  { value: "gemini-2.5-pro", label: "Gemini 2.5 Pro" },
  { value: "gemini-2.5-flash", label: "Gemini 2.5 Flash" },
  { value: "gemini-2.5-flash-lite", label: "Gemini 2.5 Flash Lite" },
  { value: "claude-haiku", label: "Claude Haiku" },
  { value: "claude-sonnet", label: "Claude Sonnet" },
  { value: "claude-opus", label: "Claude Opus" },
  { value: "gpt-5.4-mini", label: "GPT 5.4 Mini" },
  { value: "gpt-5.4", label: "GPT 5.4" },
  { value: "gpt-5.5", label: "GPT 5.5" },
] as const;

export type SupportedStepModel = string;

export interface SupportedModel {
  id: string;
  providerKey: "codex" | "claude" | "gemini";
  modelId: string;
  displayName: string;
  isEnabled: boolean;
  sortOrder: number;
  source: string;
  detectionMethod?: string | null;
  detectedCliVersion?: string | null;
  lastDetectedAt?: string | null;
  createdAt: string;
  updatedAt: string;
}

export type ReasoningEffort = "low" | "medium" | "high" | "xhigh";

export const REASONING_EFFORT_OPTIONS = [
  { value: "low", label: "Low" },
  { value: "medium", label: "Medium" },
  { value: "high", label: "High" },
  { value: "xhigh", label: "Extra High" },
] as const;

export type WorkflowStartMode = "workflow-definition" | "single-step";

const LEGACY_STEP_MODEL_ALIASES: Record<string, string> = {
  flash: "gemini-2.5-flash",
  "gemini-flash": "gemini-2.5-flash",
  pro: "gemini-2.5-pro",
  "gemini-pro": "gemini-2.5-pro",
};

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

export function normalizeStepModel(value: string | null | undefined) {
  const trimmed = value?.trim();
  if (!trimmed) {
    return null;
  }

  return LEGACY_STEP_MODEL_ALIASES[trimmed.toLowerCase()] ?? trimmed;
}

export function coerceSupportedStepModel(
  value: string | null | undefined,
  fallback: string,
): string {
  const normalized = normalizeStepModel(value);
  if (!normalized) {
    return fallback;
  }

  return normalized;
}

export function isSupportedStepModel(value: string) {
  const normalized = normalizeStepModel(value);
  return STEP_MODEL_OPTIONS.some((option) => option.value === normalized);
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
  reasoningEffort?: string | null;
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
  artifactDefinitionKey: string | null;
  workflowId: string;
  workflowRunId: string;
  workflowRunStepId: string | null;
  projectId: string | null;
  title: string;
  localPath: string;
  remotePath: string;
  remoteUrl: string;
  storageProvider: "supabase" | "google_drive" | null;
  remoteObjectId: string | null;
  syncStatus: ArtifactSyncStatus;
  replicas: ArtifactRunReplica[];
  createdAt: string;
  updatedAt: string;
}

export interface ArtifactRunReplica {
  id: string;
  artifactRunId: string;
  projectId: string | null;
  provider: "supabase" | "google_drive";
  storageScopeKey: string | null;
  remotePath: string;
  remoteObjectId: string | null;
  syncStatus: Exclude<ArtifactSyncStatus, "local_only">;
  checksum: string | null;
  lastSyncedAt: string | null;
  lastError: string | null;
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

export interface WorkflowRunSession {
  id: string;
  workflowRunId: string;
  provider: string;
  model: string;
  transportType: string;
  providerSessionId: string | null;
  processKey: string | null;
  processPid?: number | null;
  status: string;
  metadataJson?: Record<string, any> | null;
  startedAt: string;
  completedAt: string | null;
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
