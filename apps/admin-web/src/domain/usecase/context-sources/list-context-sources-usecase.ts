import type { ContextSourceGateway } from "@/domain/gateway/context-source-gateway";

export class ListContextSourcesUseCase {
  constructor(private readonly contextGateway: ContextSourceGateway) {}

  execute(projectId?: string) {
    if (!projectId) {
      return this.contextGateway.listContextSources();
    }

    return this.contextGateway.listContextSourcesByProject(projectId);
  }
}
