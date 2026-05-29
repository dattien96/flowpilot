import type {
  LocalRunnerArtifact,
  LocalRunnerBackupResult,
  LocalRunnerDirectorySelection,
  LocalRunnerFlow,
  LocalRunnerHealth,
  LocalRunnerIntegrationConnectionRequest,
  LocalRunnerIntegrationConnectionResult,
  LocalRunnerMcpBackend,
  LocalRunnerMcpBackendActionRequest,
  LocalRunnerMcpTestRequest,
  LocalRunnerMcpTestResult,
  LocalRunnerMcpTestRunSummary,
  LocalRunnerProvider,
  LocalRunnerPromptExecutionRequest,
  LocalRunnerPromptExecutionResult,
  LocalRunnerStorageDriver,
  LocalRunnerStorageDriverRequest,
  LocalRunnerSkill,
  LocalRunnerAiSessionStartRequest,
  LocalRunnerAiSessionHandle,
  LocalRunnerAiSessionMessageRequest,
} from "@/domain/model/entity/local-runner";

export interface LocalRunnerGateway {
  getHealth(): Promise<LocalRunnerHealth>;
  pickDirectory(): Promise<LocalRunnerDirectorySelection>;
  listProviders(): Promise<LocalRunnerProvider[]>;
  listSkills(): Promise<LocalRunnerSkill[]>;
  listFlows(): Promise<LocalRunnerFlow[]>;
  listArtifacts(): Promise<LocalRunnerArtifact[]>;
  getArtifactById(artifactId: string): Promise<LocalRunnerArtifact | null>;
  getStorageDriver(): Promise<LocalRunnerStorageDriver>;
  saveStorageDriver(request: LocalRunnerStorageDriverRequest): Promise<LocalRunnerStorageDriver>;
  validateStorageDriver(): Promise<LocalRunnerStorageDriver>;
  listMcpBackends(): Promise<LocalRunnerMcpBackend[]>;
  installMcpBackend(backendKey: string): Promise<LocalRunnerMcpBackend>;
  triggerMcpBackendAction(
    backendKey: string,
    request: LocalRunnerMcpBackendActionRequest,
  ): Promise<LocalRunnerMcpBackend>;
  listMcpTestRuns(
    backendKey: string,
    projectId?: string,
    integrationId?: string,
    limit?: number,
  ): Promise<LocalRunnerMcpTestRunSummary[]>;
  runMcpTest(request: LocalRunnerMcpTestRequest): Promise<LocalRunnerMcpTestResult>;
  deleteIntegrationConnection(integrationId: string): Promise<void>;
  syncArtifact(artifactId: string): Promise<LocalRunnerArtifact>;
  deleteArtifactsByWorkflowRunIds(runIds: string[]): Promise<void>;
  createBackup(scope: string, runId: string | null): Promise<LocalRunnerBackupResult>;
  triggerIntegrationConnection(
    request: LocalRunnerIntegrationConnectionRequest,
  ): Promise<LocalRunnerIntegrationConnectionResult>;
  executePrompt(
    request: LocalRunnerPromptExecutionRequest,
  ): Promise<LocalRunnerPromptExecutionResult>;
  startSession(
    request: LocalRunnerAiSessionStartRequest,
  ): Promise<LocalRunnerAiSessionHandle>;
  sendMessage(
    request: LocalRunnerAiSessionMessageRequest,
  ): Promise<LocalRunnerPromptExecutionResult>;
  closeSession(
    session: LocalRunnerAiSessionHandle,
  ): Promise<void>;
  authenticateProvider(providerName: string): Promise<void>;
  readFile(path: string): Promise<string>;
  shutdownStack(): Promise<void>;
  restartStack(): Promise<void>;
}
