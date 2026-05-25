import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";

export class AuthenticateProviderUseCase {
  constructor(private readonly localRunnerGateway: LocalRunnerGateway) {}

  execute(providerName: string) {
    return this.localRunnerGateway.authenticateProvider(providerName);
  }
}
