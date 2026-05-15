import type {
  LocalRunnerArtifact,
  LocalRunnerBackupResult,
  LocalRunnerFlow,
  LocalRunnerHealth,
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
  syncArtifact(artifactId: string): Promise<LocalRunnerArtifact>;
  createBackup(scope: string, runId: string | null): Promise<LocalRunnerBackupResult>;
  executePrompt(
    request: LocalRunnerPromptExecutionRequest,
  ): Promise<LocalRunnerPromptExecutionResult>;
}
