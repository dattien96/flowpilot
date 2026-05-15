import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";

export class CreateArtifactBackupUseCase {
  constructor(private readonly localRunnerGateway: LocalRunnerGateway) {}

  execute(scope: string, runId: string | null) {
    return this.localRunnerGateway.createBackup(scope, runId);
  }
}
