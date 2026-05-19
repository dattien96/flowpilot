import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";

export class ListLocalMcpBackendsUseCase {
  constructor(private readonly localRunnerGateway: LocalRunnerGateway) {}

  execute() {
    return this.localRunnerGateway.listMcpBackends();
  }
}
