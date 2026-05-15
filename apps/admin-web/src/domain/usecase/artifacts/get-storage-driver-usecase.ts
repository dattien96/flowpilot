import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";

export class GetStorageDriverUseCase {
  constructor(private readonly localRunnerGateway: LocalRunnerGateway) {}

  execute() {
    return this.localRunnerGateway.getStorageDriver();
  }
}
