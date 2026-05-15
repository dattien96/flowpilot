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
  authStatus: "authenticated" | "unauthenticated" | "unknown" | "missing";
  installHint: string | null;
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
  featureId: string;
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

export interface LocalRunnerPromptExecutionRequest {
  providerKey: string;
  prompt: string;
  skillIds: string[];
  flowId: string | null;
  contextSourceIds: string[];
  timeoutMs: number;
  workingDirectory: string | null;
}

export interface LocalRunnerPromptExecutionResult {
  status: "success" | "failed";
  runId: string;
  providerKey: string;
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
