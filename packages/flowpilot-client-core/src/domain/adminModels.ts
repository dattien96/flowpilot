export type ProjectPlatform = "android" | "ios" | "web" | "multi" | "nextjs" | "react";
export type ReasoningEffort = "low" | "medium" | "high" | "xhigh";
export type ArtifactStorageProvider = "supabase" | "google_drive";

export interface Project {
  id: string;
  name: string;
  description: string;
  platform: ProjectPlatform;
  repositoryUrl: string;
  directoryPath: string | null;
  status: string;
  artifactStoragePreference: ArtifactStorageProvider;
  defaultProvider: string | null;
  defaultModel: string | null;
  defaultReasoningEffort: ReasoningEffort | null;
  sessionIdleTtlMinutes: number | null;
  createdAt: string;
  updatedAt: string;
}

export interface ProjectWorkspaceBinding {
  id: string;
  projectId: string;
  localPath: string;
  label: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface Team {
  id: string;
  name: string;
  createdAt: string;
  updatedAt: string;
}

export type MemberRole = "android" | "ios" | "backend" | "frontend" | "qa" | "devops" | "ai_workflow";
export type LevelLabel = "L1_intern" | "L2_junior" | "L3_middle" | "L4_senior" | "L5_lead";

export interface TeamMember {
  id: string;
  teamId: string;
  name: string;
  email: string | null;
  jiraAccountId: string | null;
  role: MemberRole;
  levelLabel: LevelLabel;
  skillTags: string[];
  weeklyCapacityHours: number;
  createdAt: string;
  updatedAt: string;
}

export type IntegrationType = "jira" | "figma" | "google_drive" | "firebase" | "telegram";
export type IntegrationStatus = "pending" | "awaiting_oauth" | "connected" | "failed";

export interface Integration {
  id: string;
  projectId: string;
  type: IntegrationType;
  label: string;
  mcpTypeEnabled: boolean;
  configEncrypted: Record<string, unknown>;
  status: IntegrationStatus;
  lastSyncedAt: string | null;
  lastError: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface SupportedModel {
  id: string;
  providerKey: "codex" | "claude" | "gemini";
  modelId: string;
  displayName: string;
  isEnabled: boolean;
  sortOrder: number;
  source: string;
  detectionMethod: string | null;
  detectedCliVersion: string | null;
  lastDetectedAt: string | null;
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
  reasoningEffortOverride: string | null;
  yoloMode: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface WorkflowStep {
  id: string;
  workflowId: string;
  stepType: string;
  orderIndex: number;
  isEnabled: boolean;
  providerOverride: string | null;
  modelOverride: string | null;
  reasoningEffortOverride: string | null;
  requiresApproval: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface StepDefinition {
  stepType: string;
  name: string;
  description: string;
  promptBase: string | null;
  requiredMcps: string[];
  mcpAccessMode: "read_only" | "read_write";
  requiredSkills: string[];
  teamRole: string | null;
  subagent: string | null;
  model: string;
  reasoningEffort: string | null;
  yoloMode: boolean;
  agentType: "standard" | "autonomous";
  inputArtifactDefinitions: string[];
  outputArtifactDefinitions: string[];
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
  storageProvider: ArtifactStorageProvider | null;
  remoteObjectId: string | null;
  syncStatus: string;
  createdAt: string;
  updatedAt: string;
}

export interface WorkflowRun {
  id: string;
  workflowId: string;
  projectId: string;
  status: string;
  provider: string | null;
  model: string | null;
  reasoningEffort: string | null;
  yoloMode: boolean;
  startedAt: string;
  finishedAt: string | null;
  errorMessage: string | null;
}

export interface LocalRunnerProvider {
  key: string;
  label: string;
  installed: boolean;
  version: string | null;
  authStatus?: string;
  installHint: string | null;
}

export interface LocalRunnerMcpBackend {
  key: string;
  providerType: string;
  label: string;
  state: string;
  action: "install" | "verify";
  actionLabel: string;
  lastCheckedAt: string | null;
  lastError: string | null;
}

export interface LocalRunnerArtifact {
  artifactId: string;
  title: string;
  sourceKind: string;
  projectId: string;
  workflowRunId: string;
  workflowStepKey: string;
  localPath: string;
  remotePath: string;
  remoteUrl: string;
  syncStatus: string;
  createdAt: string;
  updatedAt: string;
}

export interface LocalRunnerStorageDriver {
  driverKey: string;
  enabled: boolean;
  remoteRootPath: string;
  remoteFolderName: string;
  lastValidatedAt: string | null;
  lastSyncedAt: string | null;
  lastError: string | null;
  updatedAt: string | null;
}
