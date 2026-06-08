import type {
  LocalRunnerArtifact,
  LocalRunnerArtifactHydrationRequest,
  LocalRunnerArtifactCloudSyncResult,
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
  LocalRunnerStreamOptions,
  LocalRunnerStorageDriver,
  LocalRunnerStorageDriverRequest,
  LocalRunnerSkill,
  LocalRunnerAiSessionStartRequest,
  LocalRunnerAiSessionHandle,
  LocalRunnerAiSessionMessageRequest,
  GoogleDriveMcpProviderConfigRequest,
  GoogleDriveMcpProviderConfigResponse,
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
  exportArtifactSyncBundle(artifactId: string): Promise<ArrayBuffer>;
  saveArtifactCloudSyncResult(
    artifactId: string,
    result: LocalRunnerArtifactCloudSyncResult,
  ): Promise<LocalRunnerArtifact>;
  hydrateArtifactFromRemote(
    artifactId: string,
    request: LocalRunnerArtifactHydrationRequest,
  ): Promise<LocalRunnerArtifact>;
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
    options?: LocalRunnerStreamOptions,
  ): Promise<LocalRunnerPromptExecutionResult>;
  closeSession(
    session: LocalRunnerAiSessionHandle,
  ): Promise<void>;
  authenticateProvider(providerName: string): Promise<void>;
  readFile(path: string): Promise<string>;
  shutdownStack(): Promise<void>;
  restartStack(): Promise<void>;
  ensureGoogleDriveMcpProviderConfig(
    request: GoogleDriveMcpProviderConfigRequest,
  ): Promise<GoogleDriveMcpProviderConfigResponse>;
}
