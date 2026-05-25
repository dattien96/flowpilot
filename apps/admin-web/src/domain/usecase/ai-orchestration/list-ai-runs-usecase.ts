import type {
  AiOrchestrationGateway,
  ListAiRunsFilters,
} from "@/domain/gateway/ai-orchestration-gateway";

export class ListAiRunsUseCase {
  constructor(private readonly gateway: AiOrchestrationGateway) {}

  execute(filters?: ListAiRunsFilters) {
    return this.gateway.listAiRuns(filters);
  }

  summarize(filters?: ListAiRunsFilters) {
    return this.gateway.getAiRunSummary(filters);
  }
}
