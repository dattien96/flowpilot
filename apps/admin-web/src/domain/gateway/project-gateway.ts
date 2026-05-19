import type { Project } from "@/domain/model/entity/project";
import type { Team } from "@/domain/model/entity/team";
import type { CreateProjectPayload, UpdateProjectPayload } from "@/domain/model/payload/project-payload";

export interface ProjectGateway {
  listProjects(): Promise<Project[]>;
  getProjectById(projectId: string): Promise<Project | null>;
  createProject(payload: CreateProjectPayload): Promise<Project>;
  updateProject(projectId: string, patch: Partial<UpdateProjectPayload>): Promise<Project>;
  deleteProject(projectId: string): Promise<void>;
  listTeamsByProject(projectId: string): Promise<Team[]>;
}
