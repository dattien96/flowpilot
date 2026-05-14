import type { ContextSourceGateway } from "@/domain/gateway/context-source-gateway";

export class ListContextSourcesUseCase {
  constructor(private readonly contextGateway: ContextSourceGateway) {}

  execute(projectId: string) {
    return this.contextGateway.listContextSourcesByProject(projectId);
  }
}
