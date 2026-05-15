import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";

export class ListLocalSkillsUseCase {
  constructor(private readonly localRunnerGateway: LocalRunnerGateway) {}

  execute() {
    return this.localRunnerGateway.listSkills();
  }
}
