import type {
  AiOrchestrationGateway,
  SaveAiPromptTemplateInput,
} from "@/domain/gateway/ai-orchestration-gateway";

export class SavePromptTemplateUseCase {
  constructor(private readonly gateway: AiOrchestrationGateway) {}

  execute(input: SaveAiPromptTemplateInput) {
    return this.gateway.savePromptTemplate(input);
  }
}
