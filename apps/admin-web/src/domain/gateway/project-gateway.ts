import type { Project } from "@/domain/model/entity/project";
import type { CreateProjectPayload } from "@/domain/model/payload/project-payload";

export interface ProjectGateway {
  listProjects(): Promise<Project[]>;
  getProjectById(projectId: string): Promise<Project | null>;
  createProject(payload: CreateProjectPayload): Promise<Project>;
}
