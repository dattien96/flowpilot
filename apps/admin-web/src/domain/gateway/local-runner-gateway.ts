import type {
  LocalRunnerArtifact,
  LocalRunnerBackupResult,
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
} from "@/domain/model/entity/local-runner";

export interface LocalRunnerGateway {
  getHealth(): Promise<LocalRunnerHealth>;
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
  createBackup(scope: string, runId: string | null): Promise<LocalRunnerBackupResult>;
  triggerIntegrationConnection(
    request: LocalRunnerIntegrationConnectionRequest,
  ): Promise<LocalRunnerIntegrationConnectionResult>;
  executePrompt(
    request: LocalRunnerPromptExecutionRequest,
  ): Promise<LocalRunnerPromptExecutionResult>;
}
