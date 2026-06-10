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
  accounts?: LocalRunnerProviderAccount[];
}

export interface LocalRunnerProviderModel {
  id: string;
  displayName: string;
  available: boolean;
  source: string;
}

export interface LocalRunnerProviderAccount {
  id: string;
  homePath: string;
  label: string;
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
  useProviderCli?: boolean;
  aiProviderKey?: string;
  aiModelName?: string;
  accountHomePath?: string;
  workingDirectory?: string;
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
  aiProviderKey?: string;
  aiModelName?: string;
  mcpServerName?: string;
  mcpToolUsed?: string;
  mcpFailureCode?: string;
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
  storageProvider?: string;
  remoteObjectId?: string;
  syncStatus: "local_only" | "syncing" | "synced" | "failed";
  createdAt: string;
  updatedAt: string;
  contentMarkdown: string;
  previewMarkdown: string;
  manifestPath?: string;
  promptPath?: string;
  actualPromptPath?: string;
  stdoutPath?: string;
  stderrPath?: string;
  commandPath?: string;
  contentPath?: string;
  checksum?: string;
  promptText?: string;
  actualPromptText?: string;
  stdoutText?: string;
  stderrText?: string;
  commandText?: string;
}

export interface LocalRunnerArtifactCloudSyncResult {
  storageProvider: string;
  remotePath: string;
  remoteObjectId?: string | null;
  syncStatus: "local_only" | "syncing" | "synced" | "failed";
  errorMessage?: string | null;
}

export interface LocalRunnerArtifactHydrationRequest {
  artifactId: string;
  title: string;
  sourceKind?: string;
  projectId: string;
  featureId?: string;
  workflowRunId: string;
  workflowStepKey: string;
  providerKey?: string;
  remotePath: string;
  remoteObjectId?: string;
  sourceStorageProvider: "supabase" | "google_drive";
  createdAt?: string;
  updatedAt?: string;
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
  requiredMcps?: string[];
  skillIds: string[];
  flowId: string | null;
  contextSourceIds: string[];
  timeoutMs: number;
  workingDirectory: string | null;
  allowWrite?: boolean;
  yoloMode?: boolean;
  providerAccountId?: string;
  accountHomePath?: string;
  providerAccountHomePath?: string;
  proxyUrl?: string;
  customEnv?: Record<string, string>;
}

export interface LocalRunnerPromptExecutionResult {
  status: "success" | "failed";
  runId: string;
  providerKey: string;
  modelName: string | null;
  providerSessionId?: string | null;
  command: string;
  stdoutSummary: string;
  stderrSummary: string;
  outputMarkdown: string;
  actualPromptText?: string | null;
  artifactPaths: string[];
  startedAt: string;
  completedAt: string;
  exitCode: number;
  errorMessage: string | null;
}

export interface LocalRunnerSessionStreamEvent {
  type: "chunk" | "result" | "error";
  stream?: "stdout" | "stderr" | string;
  message?: string;
  result?: LocalRunnerPromptExecutionResult;
  error?: string;
  code?: string;
  details?: string;
}

export interface LocalRunnerStreamOptions {
  onStream?: (event: LocalRunnerSessionStreamEvent) => void | Promise<void>;
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

export interface LocalRunnerAiSessionStartRequest {
  providerKey: string;
  modelName: string;
  reasoningEffort: ReasoningEffort | null;
  workingDirectory: string;
  approvalMode: string | null;
  allowWrite: boolean;
  idleTTLSeconds?: number;
  resumeProviderSessionId?: string;
  providerAccountId?: string;
  accountHomePath?: string;
  providerAccountHomePath?: string;
  proxyUrl?: string;
  customEnv?: Record<string, string>;
}

export interface LocalRunnerAiSessionHandle {
  transportType: string;
  providerSessionId: string;
  processKey: string | null;
  processPid?: number | null;
  command?: string;
}

export interface LocalRunnerAiSessionMessageRequest {
  session: LocalRunnerAiSessionHandle;
  prompt: string;
  requiredMcps?: string[];
  skillIds: string[];
  contextSourceIds: string[];
  allowWrite?: boolean;
  yoloMode?: boolean;
  accountHomePath?: string;
  idleTTLSeconds?: number | null;
}

export interface LocalRunnerGoogleDriveProxyApproval {
  id: string;
  workflowRunId?: string;
  workflowStepRunId?: string;
  processKey?: string;
  accountHomePath?: string;
  toolName: string;
  operation?: string;
  canonicalArgsJson: string;
  argumentsHash: string;
  targetSummary?: string;
  status: string;
  decisionMode: string;
  decisionComment?: string;
  requestedAt: string;
  decidedAt?: string;
  expiresAt: string;
  resultJson?: string;
  resultDriveId?: string;
  resultDriveUrl?: string;
  errorMessage?: string;
}

export interface LocalRunnerGoogleDriveProxyApprovalDecisionRequest {
  decision: "approved" | "rejected";
  comment?: string;
}


// Google Drive MCP Provider Configuration Types

export interface GoogleDriveMcpProviderConfigStatus {
  providerKey: string;
  accountHomePath: string;
  configPath: string;
  status: "not_started" | "configured" | "config_stale" | "failed";
  configKind?: "proxy" | "legacy_raw" | "unknown";
  command?: string;
  args?: string[];
  mode?: "read_only" | "read_write" | "";
  approvalMode?: string;
  lastCheckedAt?: string;
  lastError?: string;
}

export interface GoogleDriveMcpProviderConfigRequest {
  providerKey: string;
  accountHomePath: string;
  scope: "account" | "workspace";
  mode: "read_only" | "read_write";
}

export interface GoogleDriveMcpProviderConfigResponse {
  providerKey: string;
  serverName: string;
  status: string;
  changed: boolean;
  configPath: string;
  lastError?: string;
}

export interface GoogleDriveMcpStatus {
  status: string;
  configured: boolean;
  proxyMcpEnabled: boolean;
  credentialPath?: string;
  tokenPath?: string;
  credentialFileExists: boolean;
  credentialFileValid: boolean;
  tokenFileExists: boolean;
  tokenRefreshValid: boolean;
  needsAuth: boolean;
  backendPackageAvailable: boolean;
  accountId?: string;
  accountEmail?: string;
  accountSelectionRequired?: boolean;
  grantedScopes?: string[];
  missingScopes?: string[];
  accountReady: boolean;
  artifactBindingPresent?: boolean;
  artifactReady: boolean;
  mcpReadReady: boolean;
  mcpWriteReady: boolean;
  reconnectRequired?: boolean;
  missingFields?: string[];
}

export interface GoogleDriveAccountStatus {
  accountId: string;
  accountEmail?: string;
  accountSubject?: string;
  oauthClientId?: string;
  grantedScopes?: string[];
  missingScopes?: string[];
  status: string;
  projectCount: number;
  accountReady: boolean;
  mcpReadReady: boolean;
  mcpWriteReady: boolean;
  reconnectRequired?: boolean;
  connectedAt?: string;
  updatedAt?: string;
  lastError?: string;
}

export interface GoogleDriveArtifactSyncStatus {
  status: string;
  source: string;
  configured: boolean;
  clientId?: string;
  redirectUri?: string;
  hasClientSecret: boolean;
  hasPickerApiKey: boolean;
  missingFields?: string[];
}

export interface GoogleDriveWorkspaceConfigResponse {
  artifactSync: GoogleDriveArtifactSyncStatus;
  mcp: GoogleDriveMcpStatus;
  accounts?: GoogleDriveAccountStatus[];
  providerConfigs?: GoogleDriveMcpProviderConfigStatus[];
  runnerReachable: boolean;
  lastError?: string | null;
  updatedAt?: string;
}
