import type { Project } from "@/domain/model/entity/project";
import type { ProjectWorkspaceBinding } from "@/domain/model/entity/project-workspace-binding";
import type { Team } from "@/domain/model/entity/team";
import type { CreateProjectPayload, UpdateProjectPayload } from "@/domain/model/payload/project-payload";
import type {
  CreateProjectWorkspaceBindingPayload,
  UpdateProjectWorkspaceBindingPayload,
} from "@/domain/model/payload/project-workspace-binding-payload";

export interface ProjectGateway {
  listProjects(): Promise<Project[]>;
  getProjectById(projectId: string): Promise<Project | null>;
  createProject(payload: CreateProjectPayload): Promise<Project>;
  updateProject(projectId: string, patch: Partial<UpdateProjectPayload>): Promise<Project>;
  deleteProject(projectId: string): Promise<void>;
  listTeamsByProject(projectId: string): Promise<Team[]>;
  listProjectWorkspaceBindings(projectId: string): Promise<ProjectWorkspaceBinding[]>;
  createProjectWorkspaceBinding(
    projectId: string,
    payload: CreateProjectWorkspaceBindingPayload,
  ): Promise<ProjectWorkspaceBinding>;
  updateProjectWorkspaceBinding(
    bindingId: string,
    patch: Partial<UpdateProjectWorkspaceBindingPayload>,
  ): Promise<ProjectWorkspaceBinding>;
  deleteProjectWorkspaceBinding(bindingId: string): Promise<void>;
}
