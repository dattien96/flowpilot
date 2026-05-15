import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";

export class ListLocalProvidersUseCase {
  constructor(private readonly localRunnerGateway: LocalRunnerGateway) {}

  execute() {
    return this.localRunnerGateway.listProviders();
  }
}
