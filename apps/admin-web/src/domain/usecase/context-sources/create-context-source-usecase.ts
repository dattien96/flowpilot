import type { ContextSourceGateway } from "@/domain/gateway/context-source-gateway";
import type { ProjectGateway } from "@/domain/gateway/project-gateway";
import type { CreateContextSourcePayload } from "@/domain/model/payload/context-source-payload";

export class CreateContextSourceUseCase {
  constructor(
    private readonly contextGateway: ContextSourceGateway,
    private readonly projectGateway: ProjectGateway,
  ) {}

  async execute(payload: CreateContextSourcePayload) {
    const project = await this.projectGateway.getProjectById(payload.projectId);

    if (!project) {
      throw new Error("Project not found.");
    }

    return this.contextGateway.createContextSource(payload);
  }
}
