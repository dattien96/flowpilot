import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";

export class ListLocalFlowsUseCase {
  constructor(private readonly localRunnerGateway: LocalRunnerGateway) {}

  execute() {
    return this.localRunnerGateway.listFlows();
  }
}
