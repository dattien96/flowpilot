import type {
  DirectoryRepository,
  ArtifactRepository,
  IntegrationRepository,
  ProjectRepository,
  ProviderRepository,
  TeamRepository,
  WorkflowRepository,
} from "./adminRepositories";

export class AdminUseCases {
  constructor(
    readonly projects: ProjectRepository,
    readonly teams: TeamRepository,
    readonly workflows: WorkflowRepository,
    readonly artifacts: ArtifactRepository,
    readonly providers: ProviderRepository,
    readonly integrations: IntegrationRepository,
    readonly directories: DirectoryRepository,
  ) {}
}
