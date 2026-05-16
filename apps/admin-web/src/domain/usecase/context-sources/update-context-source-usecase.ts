import type { ContextSourceGateway } from "@/domain/gateway/context-source-gateway";
import type { UpdateContextSourcePayload } from "@/domain/model/payload/context-source-payload";

export class UpdateContextSourceUseCase {
  constructor(private readonly contextGateway: ContextSourceGateway) {}

  async execute(payload: UpdateContextSourcePayload) {
    const existing = await this.contextGateway.getContextSourceById(payload.contextSourceId);

    if (!existing) {
      throw new Error("Context source not found.");
    }

    return this.contextGateway.updateContextSource(payload);
  }
}
