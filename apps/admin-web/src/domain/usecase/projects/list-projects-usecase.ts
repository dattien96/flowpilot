import type { ProjectGateway } from "@/domain/gateway/project-gateway";

export class ListProjectsUseCase {
  constructor(private readonly projectGateway: ProjectGateway) {}

  execute() {
    return this.projectGateway.listProjects();
  }
}
