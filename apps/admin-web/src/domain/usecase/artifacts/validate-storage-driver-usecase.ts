import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";

export class ValidateStorageDriverUseCase {
  constructor(private readonly localRunnerGateway: LocalRunnerGateway) {}

  execute() {
    return this.localRunnerGateway.validateStorageDriver();
  }
}
