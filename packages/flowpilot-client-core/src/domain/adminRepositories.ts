import type {
  ArtifactInstance,
  ArtifactRun,
  ArtifactType,
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
  TelegramApprovalRecord,
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
  /**
   * Reverse lookup for step-definition deletion: which workflows currently
   * reference any of the given step types, across every workflow (not just
   * the one selected in the editor). Deleting a step out from under a
   * workflow that still lists it leaves that workflow's step list dangling,
   * so callers use this to cascade-delete those workflows first.
   */
  listWorkflowsUsingSteps(stepTypes: string[]): Promise<{ workflowId: string; stepType: string }[]>;
  listWorkflowRuns(projectId?: string): Promise<WorkflowRun[]>;
  /**
   * Clone a built-in (isBuiltin=true) workflow into an editable, user-owned
   * copy: a new workflow row with isBuiltin=false, editable=true,
   * clonedFrom=workflowId, plus a deep copy of its steps (CP-42/Task-179).
   */
  cloneWorkflow(workflowId: string, name: string): Promise<Workflow>;

  /** CP-45/SD-23: the system-owned, read-only artifact type catalog. */
  listArtifactTypes(): Promise<ArtifactType[]>;
  /** CP-45/SD-23: built-in (isBuiltin=true, global) + user-authored, project-scoped instances. */
  listArtifactInstances(): Promise<ArtifactInstance[]>;
  /** Creates or updates a user-authored (isBuiltin=false) instance; throws if isBuiltin is set. */
  saveArtifactInstance(
    instance: Partial<ArtifactInstance> & Pick<ArtifactInstance, "artifactTypeId" | "name">,
  ): Promise<ArtifactInstance>;
  /** Throws if the instance is built-in, or still bound to a step (Task-199 delete-guard). */
  deleteArtifactInstance(instanceId: string): Promise<void>;
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

export interface ArtifactRepository extends ArtifactRunRepository, LocalArtifactRepository, StorageDriverRepository {}

export interface LocalProviderRepository {
  listLocalProviders(): Promise<LocalRunnerProvider[]>;
  installLocalProvider(providerKey: string): Promise<LocalRunnerProvider[]>;
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
    projectId: string | null;
    type: IntegrationType;
    label: string;
    configEncrypted: Record<string, unknown>;
    status: string;
    mcpTypeEnabled?: boolean;
  }): Promise<Integration>;
  updateIntegration(id: string, patch: Partial<Integration>): Promise<Integration>;
  deleteIntegration(id: string): Promise<void>;
}

export interface ProjectIntegrationRepository {
  listLinkedIntegrations(projectId: string): Promise<Integration[]>;
  setProjectIntegration(projectId: string, type: IntegrationType, integrationId: string | null): Promise<void>;
}

/**
 * Result of a connection/test attempt against an MCP integration. `ok` is
 * false when the runner rejected the connection (e.g. a Jira verify failure),
 * so the UI can surface `message` as an error rather than as normal feedback.
 */
export interface IntegrationConnectionOutcome {
  message: string | null;
  ok: boolean;
}

export interface McpBackendRepository {
  listMcpBackends(): Promise<LocalRunnerMcpBackend[]>;
  runMcpBackendAction(backendKey: string, action: "install" | "verify", projectId?: string, integrationId?: string): Promise<void>;
  testIntegration(projectId: string | undefined, integrationId: string, providerType: string, fields?: Record<string, string | boolean | undefined>): Promise<IntegrationConnectionOutcome>;
  /** Task-233 DOD-6 revisit: real Telegram send_message approval queue. */
  listTelegramProxyApprovals(status?: string): Promise<TelegramApprovalRecord[]>;
  decideTelegramProxyApproval(id: string, decision: "approved" | "rejected", comment?: string): Promise<TelegramApprovalRecord>;
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
