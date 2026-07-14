import type {
  ArtifactRunRepository,
  IntegrationCrudRepository,
  ProjectRepository,
  ProjectIntegrationRepository,
  SupportedModelRepository,
  TeamRepository,
  WorkflowRepository,
} from "../domain/adminRepositories";
import type {
  ArtifactInstance,
  ArtifactRun,
  ArtifactType,
  Integration,
  IntegrationType,
  Project,
  ProjectPlatform,
  ProjectWorkspaceBinding,
  ReasoningEffort,
  StepArtifactBinding,
  StepDefinition,
  SupportedModel,
  Team,
  TeamMember,
  Workflow,
  WorkflowFlowEdge,
  WorkflowRun,
  WorkflowStep,
} from "../domain/adminModels";

type SupabaseLike = {
  from(table: string): any;
};

type Row = Record<string, any>;

function assertNoError(error: { message?: string } | null | undefined, fallback: string) {
  if (error) throw new Error(error.message || fallback);
}

function now() {
  return new Date().toISOString();
}

const workflowStepInsertOrderOffset = 1_000_000;

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function mapProject(row: Row): Project {
  return {
    id: String(row.id),
    name: String(row.name ?? ""),
    description: String(row.description ?? ""),
    platform: (row.platform ?? "none") as ProjectPlatform,
    repositoryUrl: String(row.repository_url ?? ""),
    directoryPath: row.directory_path ? String(row.directory_path) : null,
    status: String(row.status ?? "active"),
    artifactStoragePreference: (row.artifact_storage_preference ?? "supabase") as Project["artifactStoragePreference"],
    defaultProvider: row.default_provider ? String(row.default_provider) : null,
    defaultModel: row.default_model ? String(row.default_model) : null,
    defaultReasoningEffort: row.default_reasoning_effort ? (row.default_reasoning_effort as ReasoningEffort) : null,
    sessionIdleTtlMinutes: row.session_idle_ttl_minutes == null ? null : Number(row.session_idle_ttl_minutes),
    xcodeScheme: row.xcode_scheme ? String(row.xcode_scheme) : null,
    xcodeDestination: row.xcode_destination ? String(row.xcode_destination) : null,
    createdAt: String(row.created_at ?? ""),
    updatedAt: String(row.updated_at ?? ""),
  };
}

function mapBinding(row: Row): ProjectWorkspaceBinding {
  return {
    id: String(row.id),
    projectId: String(row.project_id),
    localPath: String(row.local_path ?? ""),
    label: row.label ? String(row.label) : null,
    createdAt: String(row.created_at ?? ""),
    updatedAt: String(row.updated_at ?? ""),
  };
}

function mapTeam(row: Row): Team {
  return {
    id: String(row.id),
    name: String(row.name ?? ""),
    createdAt: String(row.created_at ?? ""),
    updatedAt: String(row.updated_at ?? ""),
  };
}

function mapMember(row: Row): TeamMember {
  return {
    id: String(row.id),
    teamId: String(row.team_id),
    name: String(row.name ?? ""),
    email: row.email ? String(row.email) : null,
    jiraAccountId: row.jira_account_id ? String(row.jira_account_id) : null,
    role: row.role ?? "backend",
    levelLabel: row.level_label ?? "L3_middle",
    skillTags: Array.isArray(row.skill_tags) ? row.skill_tags.map(String) : [],
    weeklyCapacityHours: Number(row.weekly_capacity_hours ?? 40),
    createdAt: String(row.created_at ?? ""),
    updatedAt: String(row.updated_at ?? ""),
  };
}

function mapIntegration(row: Row): Integration {
  return {
    id: String(row.id),
    projectId: row.project_id ? String(row.project_id) : null,
    type: row.type as IntegrationType,
    label: String(row.label ?? ""),
    mcpTypeEnabled: Boolean(row.mcp_type_enabled ?? false),
    configEncrypted: row.config_encrypted && typeof row.config_encrypted === "object" ? row.config_encrypted : {},
    status: row.status ?? "pending",
    lastSyncedAt: row.last_synced_at ? String(row.last_synced_at) : null,
    lastError: row.last_error ? String(row.last_error) : null,
    createdAt: String(row.created_at ?? ""),
    updatedAt: String(row.updated_at ?? ""),
  };
}

function mapSupportedModel(row: Row): SupportedModel {
  return {
    id: String(row.id),
    providerKey: row.provider_key,
    modelId: String(row.model_id ?? ""),
    displayName: String(row.display_name ?? row.model_id ?? ""),
    isEnabled: Boolean(row.is_enabled ?? true),
    sortOrder: Number(row.sort_order ?? 0),
    source: String(row.source ?? "manual"),
    detectionMethod: row.detection_method ? String(row.detection_method) : null,
    detectedCliVersion: row.detected_cli_version ? String(row.detected_cli_version) : null,
    lastDetectedAt: row.last_detected_at ? String(row.last_detected_at) : null,
    supportedReasoningEfforts: Array.isArray(row.supported_reasoning_efforts)
      ? row.supported_reasoning_efforts.map(String)
      : null,
    defaultReasoningEffort: row.default_reasoning_effort ? String(row.default_reasoning_effort) : null,
    contextWindowTokens: row.context_window_tokens == null ? null : Number(row.context_window_tokens),
    maxContextWindowTokens: row.max_context_window_tokens == null ? null : Number(row.max_context_window_tokens),
    createdAt: String(row.created_at ?? ""),
    updatedAt: String(row.updated_at ?? ""),
  };
}

function mapWorkflow(row: Row): Workflow {
  return {
    id: String(row.id),
    projectId: row.project_id ? String(row.project_id) : null,
    name: String(row.name ?? ""),
    description: String(row.description ?? ""),
    isTemplate: Boolean(row.is_template ?? false),
    providerOverride: row.provider_override ? String(row.provider_override) : null,
    modelOverride: row.model_override ? String(row.model_override) : null,
    reasoningEffortOverride: row.reasoning_effort_override ? String(row.reasoning_effort_override) : null,
    yoloMode: Boolean(row.yolo_mode ?? false),
    createdAt: String(row.created_at ?? ""),
    updatedAt: String(row.updated_at ?? ""),
    isBuiltin: Boolean(row.is_builtin ?? false),
    editable: Boolean(row.editable ?? true),
    cloneable: Boolean(row.cloneable ?? true),
    clonedFrom: row.cloned_from ? String(row.cloned_from) : null,
    packId: row.pack_id ? String(row.pack_id) : null,
    packVersion: row.pack_version ? String(row.pack_version) : null,
    packFlowId: row.pack_flow_id ? String(row.pack_flow_id) : null,
    packHash: row.pack_hash ? String(row.pack_hash) : null,
    selectableIn: Array.isArray(row.selectable_in_json) ? row.selectable_in_json.map(String) : [],
    chatBaseline: Boolean(row.chat_baseline ?? false),
    chatSubModes: Array.isArray(row.chat_sub_modes_json) ? row.chat_sub_modes_json.map(String) : [],
    policyCap: row.policy_cap == null ? null : Number(row.policy_cap),
    policyOnCap: row.policy_on_cap ? String(row.policy_on_cap) : null,
    policyExtendBy: row.policy_extend_by == null ? null : Number(row.policy_extend_by),
    policyExtendMax: row.policy_extend_max == null ? null : Number(row.policy_extend_max),
    edges: Array.isArray(row.edges_json)
      ? row.edges_json.map(
          (edge: Row): WorkflowFlowEdge => ({
            from: String(edge.from ?? ""),
            to: String(edge.to ?? ""),
            when: String(edge.when ?? ""),
            kind: String(edge.kind ?? ""),
          }),
        )
      : [],
  };
}

function mapWorkflowStep(row: Row): WorkflowStep {
  return {
    id: String(row.id),
    workflowId: String(row.workflow_id),
    stepType: String(row.step_type ?? ""),
    orderIndex: Number(row.order_index ?? 0),
    isEnabled: Boolean(row.is_enabled ?? true),
    requiresApproval: Boolean(row.requires_approval ?? true),
    createdAt: String(row.created_at ?? ""),
    updatedAt: String(row.updated_at ?? ""),
  };
}

function mapStepDefinition(row: Row): StepDefinition {
  return {
    stepType: String(row.step_type ?? ""),
    name: String(row.name ?? ""),
    description: String(row.description ?? ""),
    promptBase: row.prompt_base ? String(row.prompt_base) : null,
    requiredMcps: Array.isArray(row.required_mcps) ? row.required_mcps.map(String) : [],
    mcpAccessMode: row.mcp_access_mode === "read_write" ? "read_write" : "read_only",
    requiredSkills: Array.isArray(row.required_skills) ? row.required_skills.map(String) : [],
    teamRole: row.team_role ? String(row.team_role) : null,
    subagent: row.subagent ? String(row.subagent) : null,
    model: String(row.model ?? "gpt-5.4"),
    reasoningEffort: row.reasoning_effort ? String(row.reasoning_effort) : null,
    yoloMode: Boolean(row.yolo_mode ?? false),
    agentType: row.agent_type === "autonomous" ? "autonomous" : "standard",
    nodeId: row.node_id ? String(row.node_id) : null,
    behaviorId: row.behavior_id ? String(row.behavior_id) : null,
    agentRef: row.agent_ref ? String(row.agent_ref) : null,
    nodeLifecycle: row.node_lifecycle ? String(row.node_lifecycle) : null,
    dependsOn: Array.isArray(row.depends_on_json) ? row.depends_on_json.map(String) : [],
    joinMode: row.join_mode ? String(row.join_mode) : null,
    cohort: row.cohort ? String(row.cohort) : null,
    promptTemplateRef: row.prompt_template_ref ? String(row.prompt_template_ref) : null,
    contextRef: row.context_ref ? String(row.context_ref) : null,
    contextSources: Array.isArray(row.context_sources) ? row.context_sources.map(String) : [],
    artifactBindings: Array.isArray(row.artifact_bindings) ? row.artifact_bindings.map(mapStepArtifactBinding) : [],
    createdAt: String(row.created_at ?? ""),
    updatedAt: String(row.updated_at ?? ""),
  };
}

function mapStepArtifactBinding(row: Row): StepArtifactBinding {
  return {
    id: String(row.id ?? ""),
    direction: row.direction === "output" ? "output" : "input",
    slotName: String(row.slot_name ?? ""),
    artifactInstanceId: String(row.artifact_instance_id ?? ""),
    required: Boolean(row.required ?? true),
    position: Number(row.position ?? 0),
    createdAt: String(row.created_at ?? ""),
  };
}

function mapArtifactType(row: Row): ArtifactType {
  return {
    id: String(row.id ?? ""),
    version: Number(row.version ?? 1),
    category: String(row.category ?? ""),
    producerBehavior: String(row.producer_behavior ?? ""),
    consumerHints: isRecord(row.consumer_hints) ? row.consumer_hints : {},
    configSchema: isRecord(row.config_schema) ? row.config_schema : {},
    renderTemplate: String(row.render_template ?? ""),
    systemOwned: Boolean(row.system_owned ?? true),
    status: String(row.status ?? "active"),
    createdAt: String(row.created_at ?? ""),
    updatedAt: String(row.updated_at ?? ""),
  };
}

function mapArtifactInstance(row: Row): ArtifactInstance {
  return {
    id: String(row.id ?? ""),
    projectId: row.project_id ? String(row.project_id) : null,
    artifactTypeId: String(row.artifact_type_id ?? ""),
    name: String(row.name ?? ""),
    description: String(row.description ?? ""),
    configJson: isRecord(row.config_json) ? row.config_json : {},
    isBuiltin: Boolean(row.is_builtin ?? false),
    status: String(row.status ?? "active"),
    createdAt: String(row.created_at ?? ""),
    updatedAt: String(row.updated_at ?? ""),
  };
}

function mapArtifactRun(row: Row): ArtifactRun {
  return {
    id: String(row.id),
    artifactDefinitionKey: row.artifact_definition_key ? String(row.artifact_definition_key) : null,
    workflowId: String(row.workflow_id ?? ""),
    workflowRunId: String(row.workflow_run_id ?? ""),
    workflowRunStepId: row.workflow_run_step_id ? String(row.workflow_run_step_id) : null,
    projectId: row.project_id ? String(row.project_id) : null,
    title: String(row.title ?? ""),
    localPath: String(row.local_path ?? ""),
    remotePath: String(row.remote_path ?? ""),
    remoteUrl: String(row.remote_url ?? ""),
    storageProvider: row.storage_provider ?? null,
    remoteObjectId: row.remote_object_id ? String(row.remote_object_id) : null,
    syncStatus: String(row.sync_status ?? "local_only"),
    createdAt: String(row.created_at ?? ""),
    updatedAt: String(row.updated_at ?? ""),
  };
}

function mapWorkflowRun(row: Row): WorkflowRun {
  return {
    id: String(row.id),
    workflowId: String(row.workflow_id ?? ""),
    projectId: String(row.project_id ?? ""),
    status: String(row.status ?? ""),
    provider: row.provider ? String(row.provider) : null,
    model: row.model ? String(row.model) : null,
    reasoningEffort: row.reasoning_effort ? String(row.reasoning_effort) : null,
    yoloMode: Boolean(row.yolo_mode ?? false),
    startedAt: String(row.started_at ?? ""),
    finishedAt: row.finished_at ? String(row.finished_at) : null,
    errorMessage: row.error_message ? String(row.error_message) : null,
  };
}

export class SupabaseAdminRepository implements
  ProjectRepository,
  TeamRepository,
  WorkflowRepository,
  SupportedModelRepository,
  IntegrationCrudRepository,
  ProjectIntegrationRepository,
  ArtifactRunRepository
{
  constructor(private readonly supabase: SupabaseLike) {}

  async listProjects() {
    const { data, error } = await this.supabase.from("projects").select("*").order("updated_at", { ascending: false });
    assertNoError(error, "Unable to list projects.");
    return (data ?? []).map(mapProject);
  }

  async createProject(input: Partial<Project> & Pick<Project, "name" | "description" | "platform" | "repositoryUrl">) {
    const legacyId = `project_${crypto.randomUUID().replaceAll("-", "").slice(0, 18)}`;
    const { data, error } = await this.supabase.from("projects").insert({
      legacy_id: legacyId,
      name: input.name,
      description: input.description,
      platform: input.platform,
      repository_url: input.repositoryUrl,
      directory_path: input.directoryPath ?? "",
      status: input.status ?? "active",
      artifact_storage_preference: input.artifactStoragePreference ?? "supabase",
      default_provider: input.defaultProvider ?? null,
      default_model: input.defaultModel ?? null,
      default_reasoning_effort: input.defaultReasoningEffort ?? null,
      session_idle_ttl_minutes: input.sessionIdleTtlMinutes ?? 120,
      xcode_scheme: input.xcodeScheme ?? null,
      xcode_destination: input.xcodeDestination ?? null,
      created_by: "supabase-admin",
    }).select("*").single();
    assertNoError(error, "Unable to create project.");
    return mapProject(data);
  }

  async updateProject(projectId: string, patch: Partial<Project>) {
    const payload: Row = {};
    if (patch.name !== undefined) payload.name = patch.name;
    if (patch.description !== undefined) payload.description = patch.description;
    if (patch.platform !== undefined) payload.platform = patch.platform;
    if (patch.repositoryUrl !== undefined) payload.repository_url = patch.repositoryUrl;
    if (patch.directoryPath !== undefined) payload.directory_path = patch.directoryPath;
    if (patch.status !== undefined) payload.status = patch.status;
    if (patch.artifactStoragePreference !== undefined) payload.artifact_storage_preference = patch.artifactStoragePreference;
    if (patch.defaultProvider !== undefined) payload.default_provider = patch.defaultProvider;
    if (patch.defaultModel !== undefined) payload.default_model = patch.defaultModel;
    if (patch.defaultReasoningEffort !== undefined) payload.default_reasoning_effort = patch.defaultReasoningEffort;
    if (patch.sessionIdleTtlMinutes !== undefined) payload.session_idle_ttl_minutes = patch.sessionIdleTtlMinutes;
    if (patch.xcodeScheme !== undefined) payload.xcode_scheme = patch.xcodeScheme;
    if (patch.xcodeDestination !== undefined) payload.xcode_destination = patch.xcodeDestination;
    payload.updated_at = now();
    const { data, error } = await this.supabase.from("projects").update(payload).eq("id", projectId).select("*").single();
    assertNoError(error, "Unable to update project.");
    return mapProject(data);
  }

  async deleteProject(projectId: string) {
    const { error } = await this.supabase.from("projects").delete().eq("id", projectId);
    assertNoError(error, "Unable to delete project.");
  }

  async listBindings(projectId: string) {
    const { data, error } = await this.supabase.from("project_workspace_bindings").select("*").eq("project_id", projectId).order("created_at", { ascending: true });
    assertNoError(error, "Unable to list directory bindings.");
    return (data ?? []).map(mapBinding);
  }

  async saveBinding(projectId: string, binding: Partial<ProjectWorkspaceBinding> & { localPath: string }) {
    const payload = {
      id: binding.id,
      project_id: projectId,
      local_path: binding.localPath,
      label: binding.label ?? null,
      updated_at: now(),
    };
    const { data, error } = await this.supabase.from("project_workspace_bindings").upsert(payload).select("*").single();
    assertNoError(error, "Unable to save directory binding.");
    return mapBinding(data);
  }

  async deleteBinding(bindingId: string) {
    const { error } = await this.supabase.from("project_workspace_bindings").delete().eq("id", bindingId);
    assertNoError(error, "Unable to delete directory binding.");
  }

  async listTeams() {
    const { data, error } = await this.supabase.from("teams").select("*").order("name", { ascending: true });
    assertNoError(error, "Unable to list teams.");
    return (data ?? []).map(mapTeam);
  }

  async createTeam(name: string) {
    const { data, error } = await this.supabase.from("teams").insert({ name }).select("*").single();
    assertNoError(error, "Unable to create team.");
    return mapTeam(data);
  }

  async updateTeam(teamId: string, name: string) {
    const { data, error } = await this.supabase
      .from("teams")
      .update({ name, updated_at: now() })
      .eq("id", teamId)
      .select("*")
      .single();
    assertNoError(error, "Unable to update team.");
    return mapTeam(data);
  }

  async deleteTeam(teamId: string) {
    const { error } = await this.supabase.from("teams").delete().eq("id", teamId);
    assertNoError(error, "Unable to delete team.");
  }

  async listMembers(teamId: string) {
    const { data, error } = await this.supabase.from("team_members").select("*").eq("team_id", teamId).order("name", { ascending: true });
    assertNoError(error, "Unable to list team members.");
    return (data ?? []).map(mapMember);
  }

  async addMember(member: Omit<TeamMember, "id" | "createdAt" | "updatedAt">) {
    const { data, error } = await this.supabase.from("team_members").insert({
      team_id: member.teamId,
      name: member.name,
      email: member.email,
      jira_account_id: member.jiraAccountId,
      role: member.role,
      level_label: member.levelLabel,
      skill_tags: member.skillTags,
      weekly_capacity_hours: member.weeklyCapacityHours,
    }).select("*").single();
    assertNoError(error, "Unable to add team member.");
    return mapMember(data);
  }

  async updateMember(memberId: string, patch: Partial<Omit<TeamMember, "id" | "teamId" | "createdAt" | "updatedAt">>) {
    const updateFields: Record<string, unknown> = { updated_at: now() };
    if (patch.name !== undefined) updateFields.name = patch.name;
    if (patch.email !== undefined) updateFields.email = patch.email;
    if (patch.jiraAccountId !== undefined) updateFields.jira_account_id = patch.jiraAccountId;
    if (patch.role !== undefined) updateFields.role = patch.role;
    if (patch.levelLabel !== undefined) updateFields.level_label = patch.levelLabel;
    if (patch.skillTags !== undefined) updateFields.skill_tags = patch.skillTags;
    if (patch.weeklyCapacityHours !== undefined) updateFields.weekly_capacity_hours = patch.weeklyCapacityHours;
    const { data, error } = await this.supabase
      .from("team_members")
      .update(updateFields)
      .eq("id", memberId)
      .select("*")
      .single();
    assertNoError(error, "Unable to update team member.");
    return mapMember(data);
  }

  async removeMember(memberId: string) {
    const { error } = await this.supabase.from("team_members").delete().eq("id", memberId);
    assertNoError(error, "Unable to remove team member.");
  }

  async listTeamsByProject(projectId: string) {
    const { data, error } = await this.supabase.from("project_teams").select("team_id, teams(*)").eq("project_id", projectId);
    assertNoError(error, "Unable to list project teams.");
    return (data ?? [])
      .map((row: Row) => {
        if (Array.isArray(row.teams)) {
          return row.teams[0] ?? null;
        }
        return row.teams ?? null;
      })
      .filter((row: Row | null): row is Row => Boolean(row))
      .map(mapTeam);
  }

  async setProjectTeams(projectId: string, teamIds: string[]) {
    const { error: deleteError } = await this.supabase.from("project_teams").delete().eq("project_id", projectId);
    assertNoError(deleteError, "Unable to update project team links.");
    if (teamIds.length === 0) return;
    // legacy_project_id is NOT NULL after the UUID baseline migration (20260519070000).
    // Fetch it from projects before inserting so the constraint is satisfied.
    const { data: projectRow, error: projectError } = await this.supabase
      .from("projects")
      .select("legacy_id")
      .eq("id", projectId)
      .single();
    assertNoError(projectError, "Unable to load project for team link.");
    const legacyProjectId: string = (projectRow as Row)?.legacy_id ?? "";
    const { error } = await this.supabase.from("project_teams").insert(
      teamIds.map((teamId) => ({ project_id: projectId, legacy_project_id: legacyProjectId, team_id: teamId }))
    );
    assertNoError(error, "Unable to update project team links.");
  }

  async listIntegrations() {
    const { data, error } = await this.supabase.from("integrations").select("*").order("updated_at", { ascending: false });
    assertNoError(error, "Unable to list integrations.");
    return (data ?? []).map(mapIntegration);
  }

  async createIntegration(input: { projectId: string | null; type: IntegrationType; label: string; configEncrypted: Record<string, unknown>; status: string; mcpTypeEnabled?: boolean }) {
    const { data, error } = await this.supabase.from("integrations").insert({
      project_id: input.projectId,
      type: input.type,
      label: input.label,
      config_encrypted: input.configEncrypted,
      status: input.status,
      mcp_type_enabled: input.mcpTypeEnabled ?? false,
    }).select("*").single();
    assertNoError(error, "Unable to create integration.");
    return mapIntegration(data);
  }

  async updateIntegration(id: string, patch: Partial<Integration>) {
    const payload: Row = {};
    if (patch.label !== undefined) payload.label = patch.label;
    if (patch.configEncrypted !== undefined) payload.config_encrypted = patch.configEncrypted;
    if (patch.status !== undefined) payload.status = patch.status;
    if (patch.mcpTypeEnabled !== undefined) payload.mcp_type_enabled = patch.mcpTypeEnabled;
    const { data, error } = await this.supabase.from("integrations").update(payload).eq("id", id).select("*").single();
    assertNoError(error, "Unable to update integration.");
    return mapIntegration(data);
  }

  async deleteIntegration(id: string) {
    const { error } = await this.supabase.from("integrations").delete().eq("id", id);
    assertNoError(error, "Unable to delete integration.");
  }

  async listLinkedIntegrations(projectId: string) {
    const { data, error } = await this.supabase.from("project_mcp_links").select("integration_id, integrations(*)").eq("project_id", projectId);
    assertNoError(error, "Unable to list linked integrations.");
    return (data ?? [])
      .map((row: Row) => {
        if (Array.isArray(row.integrations)) {
          return row.integrations[0] ?? null;
        }
        return row.integrations ?? null;
      })
      .filter((row: Row | null): row is Row => Boolean(row))
      .map(mapIntegration);
  }

  async setProjectIntegration(projectId: string, type: IntegrationType, integrationId: string | null) {
    const linked: Integration[] = await this.listLinkedIntegrations(projectId);
    await Promise.all(linked.filter((item: Integration) => item.type === type).map((item: Integration) =>
      this.supabase.from("project_mcp_links").delete().eq("project_id", projectId).eq("integration_id", item.id),
    ));
    if (!integrationId) return;
    const { error } = await this.supabase.from("project_mcp_links").insert({ project_id: projectId, integration_id: integrationId, type });
    assertNoError(error, "Unable to link integration.");
  }

  async listWorkflows() {
    const { data, error } = await this.supabase
      .from("workflows")
      .select("*")
      .neq("created_by", "flowpilot-runtime")
      .order("updated_at", { ascending: false });
    assertNoError(error, "Unable to list workflows.");
    return (data ?? []).map(mapWorkflow);
  }

  async saveWorkflow(workflow: Partial<Workflow> & { steps?: Partial<WorkflowStep>[] }) {
    if (workflow.id) {
      const { data: existing, error: existingError } = await this.supabase
        .from("workflows")
        .select("editable")
        .eq("id", workflow.id)
        .maybeSingle();
      assertNoError(existingError, "Unable to load workflow before saving.");
      if (existing && existing.editable === false) {
        const updatePayload = {
          model_override: workflow.modelOverride ?? null,
          reasoning_effort_override: workflow.reasoningEffortOverride ?? null,
          yolo_mode: workflow.yoloMode ?? false,
          updated_at: now(),
        };
        const { data, error } = await this.supabase
          .from("workflows")
          .update(updatePayload)
          .eq("id", workflow.id)
          .select("*")
          .single();
        assertNoError(error, "Unable to save built-in workflow overrides.");
        return mapWorkflow(data);
      }
    }
    const payload = {
      id: workflow.id,
      project_id: workflow.projectId ?? null,
      name: workflow.name ?? "Untitled Workflow",
      description: workflow.description ?? "",
      is_template: workflow.isTemplate ?? false,
      provider_override: workflow.providerOverride ?? null,
      model_override: workflow.modelOverride ?? null,
      reasoning_effort_override: workflow.reasoningEffortOverride ?? null,
      yolo_mode: workflow.yoloMode ?? false,
      policy_cap: workflow.policyCap ?? null,
      policy_on_cap: workflow.policyOnCap ?? null,
      policy_extend_by: workflow.policyExtendBy ?? null,
      policy_extend_max: workflow.policyExtendMax ?? null,
      edges_json: workflow.edges ?? [],
      updated_at: now(),
    };
    const { data, error } = await this.supabase.from("workflows").upsert(payload).select("*").single();
    assertNoError(error, "Unable to save workflow.");
    const saved = mapWorkflow(data);
    if (workflow.steps) {
      // BUG-NOTE-CP42 #18: this used to DELETE every existing step before
      // inserting the replacements, so an insert failure (FK/unique/network
      // error) permanently lost the old steps — the delete had already
      // committed. Insert the new rows first instead: a failed insert now
      // leaves the existing steps fully intact, and only a successful insert
      // triggers the delete that supersedes them.
      if (workflow.steps.length > 0) {
        const replacementSteps = workflow.steps.map((step, index) => ({
          workflow_id: saved.id,
          step_type: step.stepType,
          // Insert outside the live 0..N-1 range so replacement rows never
          // collide with the old rows that still occupy those slots until the
          // superseding delete runs.
          order_index: (step.orderIndex ?? index) + workflowStepInsertOrderOffset,
          is_enabled: step.isEnabled ?? true,
          requires_approval: step.requiresApproval ?? true,
        }));
        const { data: insertedSteps, error: stepsError } = await this.supabase
          .from("workflow_steps")
          .insert(replacementSteps)
          .select("id");
        assertNoError(stepsError, "Unable to save workflow steps.");
        const newStepIds = (insertedSteps ?? []).map((row: { id: string }) => row.id);
        const { error: deleteError } = await this.supabase
          .from("workflow_steps")
          .delete()
          .eq("workflow_id", saved.id)
          .not("id", "in", `(${newStepIds.join(",")})`);
        assertNoError(deleteError, "Unable to remove superseded workflow steps.");
        for (const [index, id] of newStepIds.entries()) {
          const { error: renormalizeError } = await this.supabase
            .from("workflow_steps")
            .update({ order_index: index })
            .eq("id", id);
          assertNoError(renormalizeError, "Unable to normalize workflow step order.");
        }
      } else {
        // A deliberate "save with zero steps" has no prior insert to order
        // against — the delete is the entire operation, not a supersession.
        const { error: deleteError } = await this.supabase.from("workflow_steps").delete().eq("workflow_id", saved.id);
        assertNoError(deleteError, "Unable to clear workflow steps.");
      }
    }
    return saved;
  }

  async cloneWorkflow(workflowId: string, name: string) {
    const { data: sourceRow, error: sourceError } = await this.supabase
      .from("workflows")
      .select("*")
      .eq("id", workflowId)
      .single();
    assertNoError(sourceError, "Unable to load workflow to clone.");
    const source = mapWorkflow(sourceRow);
    if (!source.cloneable) {
      throw new Error("This workflow is not cloneable.");
    }
    const { data: stepRows, error: stepsError } = await this.supabase
      .from("workflow_steps")
      .select("*")
      .eq("workflow_id", workflowId)
      .order("order_index", { ascending: true });
    assertNoError(stepsError, "Unable to load workflow steps to clone.");
    const sourceSteps = (stepRows ?? []).map(mapWorkflowStep);

    const insertPayload = {
      project_id: source.projectId,
      name,
      description: source.description,
      is_template: source.isTemplate,
      provider_override: source.providerOverride,
      model_override: source.modelOverride,
      reasoning_effort_override: source.reasoningEffortOverride,
      yolo_mode: source.yoloMode,
      is_builtin: false,
      editable: true,
      cloneable: true,
      cloned_from: source.id,
      policy_cap: source.policyCap,
      policy_on_cap: source.policyOnCap,
      policy_extend_by: source.policyExtendBy,
      policy_extend_max: source.policyExtendMax,
      edges_json: source.edges,
      updated_at: now(),
    };
    const { data: cloned, error: cloneError } = await this.supabase
      .from("workflows")
      .insert(insertPayload)
      .select("*")
      .single();
    assertNoError(cloneError, "Unable to clone workflow.");
    const clonedWorkflow = mapWorkflow(cloned);

    if (sourceSteps.length > 0) {
      // BUG-262: step_definitions is a de-duplicated catalog keyed by
      // step_type (BUG-164 — "a step type has exactly one configured
      // model"), not a per-workflow table. Reusing the source's step_type
      // here (the pre-fix behavior) meant the clone's workflow_steps row
      // pointed at the SAME step_definitions row as the source, including a
      // built-in's — so persistEdgeDerivedDependsOn's later save on the
      // clone's own edges silently overwrote the source's dependsOn too.
      // Cloning must instead give every cloned step its own fresh,
      // workflow-scoped step_type and an independent step_definitions row,
      // so the clone is genuinely isolated as the "Clone Workflow" dialog's
      // own copy promises ("Creates an editable copy... The original stays
      // unchanged").
      const stepTypes = sourceSteps.map((step: WorkflowStep) => step.stepType);
      const { data: definitionRows, error: definitionsError } = await this.supabase
        .from("step_definitions")
        .select("*")
        .in("step_type", stepTypes);
      assertNoError(definitionsError, "Unable to load step definitions to clone.");
      const definitionByStepType = new Map<string, StepDefinition>(
        (definitionRows ?? []).map((row: Row): [string, StepDefinition] => [
          String(row.step_type),
          mapStepDefinition(row),
        ]),
      );

      const stepTypeRemap = new Map<string, string>();
      const newDefinitionRows: Row[] = [];
      for (const step of sourceSteps as WorkflowStep[]) {
        const definition = definitionByStepType.get(step.stepType);
        if (!definition) continue; // BUG-236: a dangling stepType has nothing to clone
        const newStepType = `${clonedWorkflow.id}__${step.stepType}`;
        stepTypeRemap.set(step.stepType, newStepType);
        newDefinitionRows.push({
          step_type: newStepType,
          name: definition.name,
          description: definition.description,
          prompt_base: definition.promptBase,
          required_mcps: definition.requiredMcps,
          mcp_access_mode: definition.mcpAccessMode,
          required_skills: definition.requiredSkills,
          team_role: definition.teamRole,
          subagent: definition.subagent,
          model: definition.model,
          reasoning_effort: definition.reasoningEffort,
          yolo_mode: definition.yoloMode,
          agent_type: definition.agentType,
          // node_id (the flow-graph identity edges reference) is preserved
          // as-is: it only needs to be unique within the clone's own edge
          // graph, which it already is since it was copied from a valid
          // source graph. Only step_type (the storage row's own key) needs
          // to become workflow-scoped.
          node_id: definition.nodeId,
          behavior_id: definition.behaviorId,
          agent_ref: definition.agentRef,
          node_lifecycle: definition.nodeLifecycle,
          depends_on_json: definition.dependsOn,
          join_mode: definition.joinMode,
          cohort: definition.cohort,
          prompt_template_ref: definition.promptTemplateRef,
          context_ref: definition.contextRef,
          context_sources: definition.contextSources,
          updated_at: now(),
        });
      }
      if (newDefinitionRows.length > 0) {
        const { error: insertDefinitionsError } = await this.supabase
          .from("step_definitions")
          .insert(newDefinitionRows);
        assertNoError(insertDefinitionsError, "Unable to clone step definitions.");
      }

      const { error: insertStepsError } = await this.supabase.from("workflow_steps").insert(
        sourceSteps
          .filter((step: WorkflowStep) => stepTypeRemap.has(step.stepType))
          .map((step: WorkflowStep) => ({
            workflow_id: clonedWorkflow.id,
            step_type: stepTypeRemap.get(step.stepType)!,
            order_index: step.orderIndex,
            is_enabled: step.isEnabled,
            requires_approval: step.requiresApproval,
          })),
      );
      assertNoError(insertStepsError, "Unable to clone workflow steps.");
    }
    return clonedWorkflow;
  }

  async deleteWorkflow(workflowId: string) {
    const { error } = await this.supabase.from("workflows").delete().eq("id", workflowId);
    assertNoError(error, "Unable to delete workflow.");
  }

  async listWorkflowSteps(workflowId: string) {
    const { data, error } = await this.supabase.from("workflow_steps").select("*").eq("workflow_id", workflowId).order("order_index", { ascending: true });
    assertNoError(error, "Unable to list workflow steps.");
    return (data ?? []).map(mapWorkflowStep);
  }

  async listWorkflowsUsingSteps(stepTypes: string[]) {
    if (stepTypes.length === 0) return [];
    const { data, error } = await this.supabase
      .from("workflow_steps")
      .select("workflow_id, step_type")
      .in("step_type", stepTypes);
    assertNoError(error, "Unable to list workflows using these step types.");
    return (data ?? []).map((row: Row) => ({
      workflowId: String(row.workflow_id),
      stepType: String(row.step_type),
    }));
  }

  async listStepDefinitions() {
    const [definitionsResult, artifactBindingsResult] = await Promise.all([
      this.supabase.from("step_definitions").select("*").order("name", { ascending: true }),
      this.supabase
        .from("step_artifact_bindings")
        .select("*")
        .order("position", { ascending: true }),
    ]);
    assertNoError(definitionsResult.error, "Unable to list step definitions.");
    assertNoError(artifactBindingsResult.error, "Unable to list step artifact bindings.");

    const artifactBindings = new Map<string, Row[]>();
    for (const row of artifactBindingsResult.data ?? []) {
      const stepType = String(row.step_definition_id);
      const current = artifactBindings.get(stepType) ?? [];
      current.push(row);
      artifactBindings.set(stepType, current);
    }

    return (definitionsResult.data ?? []).map((row: Row) =>
      mapStepDefinition({
        ...row,
        artifact_bindings: artifactBindings.get(String(row.step_type)) ?? [],
      }),
    );
  }

  async saveStepDefinition(step: StepDefinition) {
    const { data, error } = await this.supabase.from("step_definitions").upsert({
      step_type: step.stepType,
      name: step.name,
      description: step.description,
      prompt_base: step.promptBase,
      required_mcps: step.requiredMcps,
      mcp_access_mode: step.mcpAccessMode,
      required_skills: step.requiredSkills,
      team_role: step.teamRole,
      subagent: step.subagent,
      model: step.model,
      reasoning_effort: step.reasoningEffort,
      yolo_mode: step.yoloMode,
      agent_type: step.agentType,
      node_id: step.nodeId ?? null,
      behavior_id: step.behaviorId ?? null,
      agent_ref: step.agentRef ?? null,
      node_lifecycle: step.nodeLifecycle ?? null,
      depends_on_json: step.dependsOn ?? [],
      join_mode: step.joinMode ?? null,
      cohort: step.cohort ?? null,
      prompt_template_ref: step.promptTemplateRef ?? null,
      context_ref: step.contextRef ?? null,
      context_sources: step.contextSources ?? [],
      updated_at: now(),
    }, { onConflict: "step_type" }).select("*").single();
    assertNoError(error, "Unable to save step definition.");

    const { error: deleteArtifactBindingsError } = await this.supabase
      .from("step_artifact_bindings")
      .delete()
      .eq("step_definition_id", step.stepType);
    assertNoError(deleteArtifactBindingsError, "Unable to reset step artifact bindings.");

    if (step.artifactBindings.length > 0) {
      const { error: artifactBindingsError } = await this.supabase
        .from("step_artifact_bindings")
        .insert(
          step.artifactBindings.map((binding, index) => ({
            step_definition_id: step.stepType,
            direction: binding.direction,
            slot_name: binding.slotName,
            artifact_instance_id: binding.artifactInstanceId,
            required: binding.required,
            position: binding.position ?? index,
          })),
        );
      assertNoError(artifactBindingsError, "Unable to save step artifact bindings.");
    }

    return mapStepDefinition({
      ...data,
      artifact_bindings: step.artifactBindings.map((binding) => ({
        id: binding.id,
        direction: binding.direction,
        slot_name: binding.slotName,
        artifact_instance_id: binding.artifactInstanceId,
        required: binding.required,
        position: binding.position,
        created_at: binding.createdAt,
      })),
    });
  }

  async deleteStepDefinition(stepType: string) {
    const { error: deleteArtifactBindingsError } = await this.supabase
      .from("step_artifact_bindings")
      .delete()
      .eq("step_definition_id", stepType);
    assertNoError(deleteArtifactBindingsError, "Unable to delete step artifact bindings.");

    const { error } = await this.supabase.from("step_definitions").delete().eq("step_type", stepType);
    assertNoError(error, "Unable to delete step definition.");
  }

  async listArtifactTypes() {
    const { data, error } = await this.supabase.from("artifact_types").select("*").order("id", { ascending: true });
    assertNoError(error, "Unable to list artifact types.");
    return (data ?? []).map(mapArtifactType);
  }

  async listArtifactInstances() {
    const { data, error } = await this.supabase
      .from("artifact_instances")
      .select("*")
      .order("is_builtin", { ascending: false })
      .order("name", { ascending: true });
    assertNoError(error, "Unable to list artifact instances.");
    return (data ?? []).map(mapArtifactInstance);
  }

  async saveArtifactInstance(instance: Partial<ArtifactInstance> & Pick<ArtifactInstance, "artifactTypeId" | "name">) {
    if (instance.isBuiltin) {
      throw new Error("Built-in artifact instances cannot be created or edited from Settings.");
    }
    const { data, error } = await this.supabase
      .from("artifact_instances")
      .upsert(
        {
          ...(instance.id ? { id: instance.id } : {}),
          project_id: instance.projectId ?? null,
          artifact_type_id: instance.artifactTypeId,
          name: instance.name,
          description: instance.description ?? "",
          config_json: instance.configJson ?? {},
          is_builtin: false,
          status: instance.status ?? "active",
          updated_at: now(),
        },
        instance.id ? { onConflict: "id" } : undefined,
      )
      .select("*")
      .single();
    assertNoError(error, "Unable to save artifact instance.");
    return mapArtifactInstance(data);
  }

  async deleteArtifactInstance(instanceId: string) {
    const { data: bound, error: boundError } = await this.supabase
      .from("step_artifact_bindings")
      .select("id")
      .eq("artifact_instance_id", instanceId)
      .limit(1);
    assertNoError(boundError, "Unable to check artifact instance bindings.");
    if ((bound ?? []).length > 0) {
      throw new Error("This artifact instance is still bound to a step and cannot be deleted.");
    }

    const { error } = await this.supabase
      .from("artifact_instances")
      .delete()
      .eq("id", instanceId)
      .eq("is_builtin", false);
    assertNoError(error, "Unable to delete artifact instance.");
  }

  async listWorkflowRuns(projectId?: string) {
    let query = this.supabase.from("workflow_runs").select("*").order("started_at", { ascending: false });
    if (projectId) query = query.eq("project_id", projectId);
    const { data, error } = await query;
    assertNoError(error, "Unable to list workflow runs.");
    return (data ?? []).map(mapWorkflowRun);
  }

  async listRuns(projectId?: string) {
    let query = this.supabase.from("artifact_runs").select("*").order("updated_at", { ascending: false });
    if (projectId) query = query.eq("project_id", projectId);
    const { data, error } = await query;
    assertNoError(error, "Unable to list artifact runs.");
    return (data ?? []).map(mapArtifactRun);
  }

  async listSupportedModels() {
    const { data, error } = await this.supabase.from("ai_supported_models").select("*").order("sort_order", { ascending: true });
    assertNoError(error, "Unable to list supported models.");
    return (data ?? []).map(mapSupportedModel);
  }

  async createSupportedModel(model: Omit<SupportedModel, "id" | "createdAt" | "updatedAt">) {
    const { data, error } = await this.supabase.from("ai_supported_models").insert({
      provider_key: model.providerKey,
      model_id: model.modelId,
      display_name: model.displayName,
      is_enabled: model.isEnabled,
      sort_order: model.sortOrder,
      source: model.source,
      detection_method: model.detectionMethod,
      detected_cli_version: model.detectedCliVersion,
      last_detected_at: model.lastDetectedAt,
      supported_reasoning_efforts: model.supportedReasoningEfforts,
      default_reasoning_effort: model.defaultReasoningEffort,
      context_window_tokens: model.contextWindowTokens,
      max_context_window_tokens: model.maxContextWindowTokens,
    }).select("*").single();
    assertNoError(error, "Unable to create supported model.");
    return mapSupportedModel(data);
  }

  async updateSupportedModel(id: string, patch: Partial<SupportedModel>) {
    const payload: Row = {};
    if (patch.modelId !== undefined) payload.model_id = patch.modelId;
    if (patch.displayName !== undefined) payload.display_name = patch.displayName;
    if (patch.isEnabled !== undefined) payload.is_enabled = patch.isEnabled;
    if (patch.sortOrder !== undefined) payload.sort_order = patch.sortOrder;
    // Task-215: re-detect refreshes reasoning/context-window provenance on an
    // already-registered row (unlike model rows themselves, which are
    // insert-only per Task-213 T-3) — these are capability facts about the
    // model, not user-editable state.
    if (patch.supportedReasoningEfforts !== undefined) payload.supported_reasoning_efforts = patch.supportedReasoningEfforts;
    if (patch.defaultReasoningEffort !== undefined) payload.default_reasoning_effort = patch.defaultReasoningEffort;
    if (patch.contextWindowTokens !== undefined) payload.context_window_tokens = patch.contextWindowTokens;
    if (patch.maxContextWindowTokens !== undefined) payload.max_context_window_tokens = patch.maxContextWindowTokens;
    const { data, error } = await this.supabase.from("ai_supported_models").update(payload).eq("id", id).select("*").single();
    assertNoError(error, "Unable to update supported model.");
    return mapSupportedModel(data);
  }

  async deleteSupportedModel(id: string) {
    const { error } = await this.supabase.from("ai_supported_models").delete().eq("id", id);
    assertNoError(error, "Unable to delete supported model.");
  }

}
