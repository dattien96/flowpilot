import type { ProjectGateway } from "@/domain/gateway/project-gateway";
import type { CreateProjectPayload } from "@/domain/model/payload/project-payload";

export class CreateProjectUseCase {
  constructor(private readonly projectGateway: ProjectGateway) {}

  execute(payload: CreateProjectPayload) {
    return this.projectGateway.createProject(payload);
  }
}
