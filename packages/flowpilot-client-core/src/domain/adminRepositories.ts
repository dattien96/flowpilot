import type {
  ArtifactDefinition,
  ArtifactRun,
  Integration,
  IntegrationType,
  LocalRunnerArtifact,
  LocalRunnerMcpBackend,
  LocalRunnerProvider,
  LocalRunnerStorageDriver,
  Project,
  ProjectWorkspaceBinding,
  SupportedModel,
  Team,
  TeamMember,
  Workflow,
  WorkflowRun,
  WorkflowStep,
  StepDefinition,
} from "./adminModels";

export interface ProjectRepository {
  listProjects(): Promise<Project[]>;
  createProject(input: Partial<Project> & Pick<Project, "name" | "description" | "platform" | "repositoryUrl">): Promise<Project>;
  updateProject(projectId: string, patch: Partial<Project>): Promise<Project>;
  deleteProject(projectId: string): Promise<void>;
  listBindings(projectId: string): Promise<ProjectWorkspaceBinding[]>;
  saveBinding(projectId: string, binding: Partial<ProjectWorkspaceBinding> & { localPath: string }): Promise<ProjectWorkspaceBinding>;
  deleteBinding(bindingId: string): Promise<void>;
}

export interface TeamRepository {
  listTeams(): Promise<Team[]>;
  createTeam(name: string): Promise<Team>;
  updateTeam(teamId: string, name: string): Promise<Team>;
  deleteTeam(teamId: string): Promise<void>;
  listMembers(teamId: string): Promise<TeamMember[]>;
  addMember(member: Omit<TeamMember, "id" | "createdAt" | "updatedAt">): Promise<TeamMember>;
  updateMember(memberId: string, patch: Partial<Omit<TeamMember, "id" | "teamId" | "createdAt" | "updatedAt">>): Promise<TeamMember>;
  removeMember(memberId: string): Promise<void>;
  listTeamsByProject(projectId: string): Promise<Team[]>;
  setProjectTeams(projectId: string, teamIds: string[]): Promise<void>;
}

export interface WorkflowRepository {
  listWorkflows(): Promise<Workflow[]>;
  saveWorkflow(workflow: Partial<Workflow> & { steps?: Partial<WorkflowStep>[] }): Promise<Workflow>;
  deleteWorkflow(workflowId: string): Promise<void>;
  listWorkflowSteps(workflowId: string): Promise<WorkflowStep[]>;
  listStepDefinitions(): Promise<StepDefinition[]>;
  saveStepDefinition(step: StepDefinition): Promise<StepDefinition>;
  deleteStepDefinition(stepType: string): Promise<void>;
  listWorkflowRuns(projectId?: string): Promise<WorkflowRun[]>;
}

export interface ArtifactCatalogRepository {
  listDefinitions(): Promise<ArtifactDefinition[]>;
  saveDefinition(definition: ArtifactDefinition): Promise<ArtifactDefinition>;
}

export interface ArtifactRunRepository {
  listRuns(projectId?: string): Promise<ArtifactRun[]>;
}

export interface LocalArtifactRepository {
  listLocalArtifacts(): Promise<LocalRunnerArtifact[]>;
}

export interface StorageDriverRepository {
  getStorageDriver(): Promise<LocalRunnerStorageDriver>;
  saveStorageDriver(driver: Pick<LocalRunnerStorageDriver, "driverKey" | "enabled" | "remoteRootPath" | "remoteFolderName">): Promise<LocalRunnerStorageDriver>;
}

export interface ArtifactRepository extends ArtifactCatalogRepository, ArtifactRunRepository, LocalArtifactRepository, StorageDriverRepository {}

export interface LocalProviderRepository {
  listLocalProviders(): Promise<LocalRunnerProvider[]>;
  authenticateProvider(providerKey: string): Promise<void>;
}

export interface SupportedModelRepository {
  listSupportedModels(): Promise<SupportedModel[]>;
  createSupportedModel(model: Omit<SupportedModel, "id" | "createdAt" | "updatedAt">): Promise<SupportedModel>;
  updateSupportedModel(id: string, patch: Partial<SupportedModel>): Promise<SupportedModel>;
  deleteSupportedModel(id: string): Promise<void>;
}

export interface ProviderRepository extends LocalProviderRepository, SupportedModelRepository {}

export interface IntegrationCrudRepository {
  listIntegrations(): Promise<Integration[]>;
  createIntegration(input: {
    projectId: string;
    type: IntegrationType;
    label: string;
    configEncrypted: Record<string, unknown>;
    status: string;
    mcpTypeEnabled?: boolean;
  }): Promise<Integration>;
  updateIntegration(id: string, patch: Partial<Integration>): Promise<Integration>;
}

export interface ProjectIntegrationRepository {
  listLinkedIntegrations(projectId: string): Promise<Integration[]>;
  setProjectIntegration(projectId: string, type: IntegrationType, integrationId: string | null): Promise<void>;
}

export interface McpBackendRepository {
  listMcpBackends(): Promise<LocalRunnerMcpBackend[]>;
  runMcpBackendAction(backendKey: string, action: "install" | "verify", projectId: string, integrationId?: string): Promise<void>;
  testIntegration(projectId: string, integrationId: string, providerType: string, fields?: Record<string, string | undefined>): Promise<string | null>;
}

export interface IntegrationRepository extends IntegrationCrudRepository, ProjectIntegrationRepository, McpBackendRepository {}

export interface DirectoryValidationResult {
  path: string;
  usable: boolean;
  reason: string;
}

export interface DirectorySelectionResult {
  path: string;
}

export interface DirectoryRepository {
  pickDirectory(): Promise<DirectorySelectionResult>;
  validatePath(path: string): Promise<DirectoryValidationResult>;
}
