import type { AiOrchestrationGateway } from "@/domain/gateway/ai-orchestration-gateway";

export class ListPromptTemplatesUseCase {
  constructor(private readonly gateway: AiOrchestrationGateway) {}

  execute(projectId?: string) {
    return this.gateway.listPromptTemplates(projectId);
  }
}
