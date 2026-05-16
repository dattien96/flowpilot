import type { ContextSourceGateway } from "@/domain/gateway/context-source-gateway";

export class DeleteContextSourceUseCase {
  constructor(private readonly contextGateway: ContextSourceGateway) {}

  async execute(contextSourceId: string) {
    const existing = await this.contextGateway.getContextSourceById(contextSourceId);

    if (!existing) {
      throw new Error("Context source not found.");
    }

    await this.contextGateway.deleteContextSource(contextSourceId);
  }
}
