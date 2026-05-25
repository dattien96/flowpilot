import type { ReasoningEffort } from "@/domain/model/entity/workflow-engine";

export interface LocalRunnerHealth {
  status: "online" | "offline";
  runnerVersion: string | null;
  cwd: string | null;
  os: string | null;
  startedAt: string | null;
  baseUrl: string;
  errorMessage: string | null;
}

export interface LocalRunnerProvider {
  key: string;
  label: string;
  installed: boolean;
  version: string | null;
  binaryPath: string | null;
  supported?: boolean;
  installStatus?: "NOT_INSTALLED" | "INSTALLED" | "FAILED" | "UNSUPPORTED_OS";
  authStatus?: "READY" | "AUTH_REQUIRED" | "UNKNOWN";
  detectedBinary?: string | null;
  detectedVersion?: string | null;
  models?: LocalRunnerProviderModel[];
  lastError?: string | null;
  installHint: string | null;
}

export interface LocalRunnerProviderModel {
  id: string;
  displayName: string;
  available: boolean;
  source: string;
}

export interface LocalRunnerMcpBackend {
  key: string;
  providerType: string;
  label: string;
  transport: "launcher" | "remote";
  state: "installed" | "launcher_available" | "missing";
  launcher: string;
  installed: boolean;
  binaryPath: string | null;
  command: string;
  installHint: string | null;
  action: "install" | "verify";
  actionLabel: string;
  lastCheckedAt: string | null;
  lastError: string | null;
}

export interface LocalRunnerMcpBackendActionRequest {
  projectId: string;
  integrationId?: string;
  action: "install" | "verify";
}

export interface LocalRunnerMcpTestRequest {
  backendKey: string;
  providerType: string;
  projectId: string;
  integrationId: string;
  templateKey: string;
  allowWrite: boolean;
  prompt: string;
  timeoutMs: number;
}

export interface LocalRunnerMcpTestResult {
  status: "success" | "failed";
  runId: string;
  backendKey: string;
  providerType: string;
  projectId: string;
  integrationId: string;
  command: string;
  stdoutSummary: string;
  stderrSummary: string;
  outputMarkdown: string;
  artifactPaths: string[];
  startedAt: string;
  completedAt: string;
  errorMessage: string | null;
}

export interface LocalRunnerMcpTestRunSummary {
  runId: string;
  backendKey: string;
  providerType: string;
  projectId: string;
  integrationId: string;
  status: "success" | "failed";
  startedAt: string;
  completedAt: string;
  artifactDir: string;
}

export interface LocalRunnerSkill {
  id: string;
  name: string;
  filePath: string;
  description: string;
  tags: string[];
}

export interface LocalRunnerFlow {
  id: string;
  name: string;
  filePath: string;
  description: string;
  steps: string[];
}

export interface LocalRunnerArtifact {
  artifactId: string;
  title: string;
  sourceKind: string;
  projectId: string;
  workflowRunId: string;
  workflowStepKey: string;
  providerKey: string;
  localPath: string;
  remotePath: string;
  remoteUrl: string;
  syncStatus: "local_only" | "syncing" | "synced" | "failed";
  createdAt: string;
  updatedAt: string;
  contentMarkdown: string;
  previewMarkdown: string;
  manifestPath?: string;
  promptPath?: string;
  stdoutPath?: string;
  stderrPath?: string;
  commandPath?: string;
  contentPath?: string;
  checksum?: string;
  promptText?: string;
  stdoutText?: string;
  stderrText?: string;
  commandText?: string;
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

export interface LocalRunnerBackupResult {
  backupPath: string;
  archiveName: string;
  createdAt: string;
}

export interface LocalRunnerStorageDriverRequest {
  driverKey: string;
  enabled: boolean;
  remoteRootPath: string;
  remoteFolderName: string;
}

export interface LocalRunnerDirectorySelection {
  path: string;
}

export interface LocalRunnerPromptExecutionRequest {
  providerKey: string;
  modelName?: string;
  reasoningEffort?: ReasoningEffort | null;
  prompt: string;
  skillIds: string[];
  flowId: string | null;
  contextSourceIds: string[];
  timeoutMs: number;
  workingDirectory: string | null;
  allowWrite?: boolean;
}

export interface LocalRunnerPromptExecutionResult {
  status: "success" | "failed";
  runId: string;
  providerKey: string;
  modelName: string | null;
  command: string;
  stdoutSummary: string;
  stderrSummary: string;
  outputMarkdown: string;
  artifactPaths: string[];
  startedAt: string;
  completedAt: string;
  exitCode: number;
  errorMessage: string | null;
}

export interface LocalRunnerIntegrationConnectionRequest {
  projectId: string;
  integrationId: string;
  providerType: string;
  action: "test" | "retry";
  email?: string;
  apiToken?: string;
}

export interface LocalRunnerIntegrationConnectionResult {
  requestStatus: "accepted" | "rejected";
  integrationId: string;
  integrationStatus: "pending" | "awaiting_oauth" | "connected" | "failed";
  runId: string | null;
  message: string | null;
}
