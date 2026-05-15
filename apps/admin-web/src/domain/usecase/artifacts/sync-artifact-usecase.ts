import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";

export class SyncArtifactUseCase {
  constructor(private readonly localRunnerGateway: LocalRunnerGateway) {}

  execute(artifactId: string) {
    return this.localRunnerGateway.syncArtifact(artifactId);
  }
}
