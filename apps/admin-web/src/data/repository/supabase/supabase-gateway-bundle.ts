import type { SupabaseClient } from "@supabase/supabase-js";

import { MockWorkflowExecutor } from "@/data/workflow/mock-workflow-executor";
import type { ContextSourceGateway } from "@/domain/gateway/context-source-gateway";
import type { IntegrationGateway } from "@/domain/gateway/integration-gateway";
import type { ProjectGateway } from "@/domain/gateway/project-gateway";
import type { TeamGateway } from "@/domain/gateway/team-gateway";
import type {
  WorkflowExecutorGateway,
  WorkflowGateway,
} from "@/domain/gateway/workflow-gateway";
import type { ContextSource } from "@/domain/model/entity/context-source";
import type { Integration } from "@/domain/model/entity/integration";
import type { Project } from "@/domain/model/entity/project";
import type { ProjectWorkspaceBinding } from "@/domain/model/entity/project-workspace-binding";
import type { Team, TeamMember } from "@/domain/model/entity/team";
import type {
  AiCallLog,
  AiOutput,
  Approval,
  ApprovalDecision,
  WorkflowDefinition,
  WorkflowDefinitionStep,
  WorkflowRun,
  WorkflowStep,
} from "@/domain/model/entity/workflow";
import type { WorkflowRunSession } from "@/domain/model/entity/workflow-engine";
import type {
  CreateContextSourcePayload,
  UpdateContextSourcePayload,
} from "@/domain/model/payload/context-source-payload";
import type {
  CreateIntegrationPayload,
  UpdateIntegrationPayload,
} from "@/domain/model/payload/integration-payload";
import type { CreateProjectPayload, UpdateProjectPayload } from "@/domain/model/payload/project-payload";
import type {
  CreateProjectWorkspaceBindingPayload,
  UpdateProjectWorkspaceBindingPayload,
} from "@/domain/model/payload/project-workspace-binding-payload";
import type { StartWorkflowRunPayload } from "@/domain/model/payload/workflow-payload";
import type { ListOutputsFilters } from "@/domain/model/payload/workflow-payload";

type SupabaseRow = Record<string, unknown>;

function createId(prefix: string) {
  return `${prefix}_${crypto.randomUUID().replaceAll("-", "").slice(0, 18)}`;
}

function createUuid() {
  return crypto.randomUUID();
}

function resolveProviderKeyFromModel(model: string) {
  if (model.startsWith("gpt-")) {
    return "codex";
  }
  if (model.startsWith("gemini-")) {
    return "gemini";
  }
  if (model.startsWith("claude-")) {
    return "claude";
  }

  return "";
}

function assertRow<T>(row: T | null, message: string): T {
  if (!row) {
    throw new Error(message);
  }

  return row;
}

function assertNoError(error: { message: string } | null, fallback: string) {
  if (error) {
    throw new Error(error.message || fallback);
  }
}

function mapProject(row: SupabaseRow): Project {
  return {
    id: String(row.id),
    name: String(row.name),
    description: String(row.description),
    platform: row.platform as Project["platform"],
    repositoryUrl: String(row.repository_url),
    directoryPath: row.directory_path ? String(row.directory_path) : null,
    ownerId: row.owner_id ? String(row.owner_id) : null,
    status: String(row.status ?? "active"),
    artifactStoragePreference: (row.artifact_storage_preference ?? "supabase") as Project["artifactStoragePreference"],
    defaultProvider: row.default_provider ? String(row.default_provider) : null,
    defaultModel: row.default_model ? String(row.default_model) : null,
    defaultReasoningEffort: row.default_reasoning_effort ? (row.default_reasoning_effort as Project["defaultReasoningEffort"]) : null,
    sessionIdleTtlMinutes: row.session_idle_ttl_minutes ? Number(row.session_idle_ttl_minutes) : null,
    createdBy: String(row.created_by),
    createdAt: String(row.created_at),
    updatedAt: String(row.updated_at),
  };
}

function buildProjectUpdatePayload(patch: Partial<UpdateProjectPayload>) {
  const nextModel = patch.defaultModel?.trim() || null;
  return Object.fromEntries(
    Object.entries({
      name: patch.name,
      description: patch.description,
      platform: patch.platform,
      repository_url: patch.repositoryUrl,
      directory_path: patch.directoryPath,
      owner_id: patch.ownerId,
      status: patch.status,
      artifact_storage_preference: patch.artifactStoragePreference,
      default_provider: nextModel ? resolveProviderKeyFromModel(nextModel) : patch.defaultProvider,
      default_model: nextModel,
      default_reasoning_effort: patch.defaultReasoningEffort,
      session_idle_ttl_minutes: patch.sessionIdleTtlMinutes,
    }).filter(([, value]) => value !== undefined),
  );
}

function mapProjectWorkspaceBinding(row: SupabaseRow): ProjectWorkspaceBinding {
  return {
    id: String(row.id),
    projectId: String(row.project_id),
    localPath: String(row.local_path),
    label: row.label ? String(row.label) : null,
    createdAt: String(row.created_at),
    updatedAt: String(row.updated_at),
  };
}

function mapTeam(row: SupabaseRow): Team {
  return {
    id: String(row.id),
    name: String(row.name),
    createdAt: String(row.created_at),
    updatedAt: String(row.updated_at),
  };
}

function mapTeamMember(row: SupabaseRow): TeamMember {
  return {
    id: String(row.id),
    teamId: String(row.team_id),
    name: String(row.name),
    email: row.email ? String(row.email) : null,
    jiraAccountId: row.jira_account_id ? String(row.jira_account_id) : null,
    role: row.role as TeamMember["role"],
    levelLabel: row.level_label as TeamMember["levelLabel"],
    skillTags: Array.isArray(row.skill_tags) ? row.skill_tags.map(String) : [],
    weeklyCapacityHours: Number(row.weekly_capacity_hours ?? 40),
    createdAt: String(row.created_at),
    updatedAt: String(row.updated_at),
  };
}

function mapIntegration(row: SupabaseRow): Integration {
  return {
    id: String(row.id),
    projectId: String(row.project_id),
    type: row.type as Integration["type"],
    label: String(row.label ?? ""),
    mcpTypeEnabled: Boolean(row.mcp_type_enabled ?? false),
    configEncrypted:
      row.config_encrypted && typeof row.config_encrypted === "object"
        ? (row.config_encrypted as Record<string, unknown>)
        : {},
    status: row.status as Integration["status"],
    lastSyncedAt: row.last_synced_at ? String(row.last_synced_at) : null,
    lastError: row.last_error ? String(row.last_error) : null,
    createdAt: String(row.created_at),
    updatedAt: String(row.updated_at),
  };
}

function mapContextSource(row: SupabaseRow): ContextSource {
  return {
    id: String(row.id),
    projectId: String(row.project_id),
    type: row.type as ContextSource["type"],
    title: String(row.title),
    rawContent: String(row.raw_content),
    summarizedContent: row.summarized_content ? String(row.summarized_content) : null,
    archivedAt: row.archived_at ? String(row.archived_at) : null,
    createdBy: String(row.created_by),
    createdAt: String(row.created_at),
  };
}

function normalizeDefinitionSteps(definition: unknown): WorkflowDefinitionStep[] {
  if (!definition || typeof definition !== "object") {
    return [];
  }

  const steps = Array.isArray((definition as { steps?: unknown }).steps)
    ? (definition as { steps: unknown[] }).steps
    : [];

  return steps.map((step) => {
    const value = step as Record<string, unknown>;
    return {
      key: String(value.key),
      name: String(value.name),
      type: value.type as WorkflowDefinitionStep["type"],
      outputType: value.outputType as WorkflowDefinitionStep["outputType"],
    };
  });
}

function mapWorkflowDefinition(row: SupabaseRow): WorkflowDefinition {
  return {
    id: String(row.id),
    name: String(row.name),
    description: String(row.description),
    version: Number(row.version),
    status: row.status as WorkflowDefinition["status"],
    steps: normalizeDefinitionSteps(row.definition),
  };
}

const APPROVAL_DECISION_LOG_PREFIX = "approval_decision:";

function workflowDefinitionStepType(
  rawStepType: unknown,
  requiresApproval: unknown,
): WorkflowDefinitionStep["type"] {
  if (Boolean(requiresApproval)) {
    return "approval";
  }

  const normalized = String(rawStepType ?? "").trim().toLowerCase();
  if (normalized === "tool") {
    return "tool";
  }

  return normalized.includes("approval") ? "approval" : "ai_mock";
}

function mapCanonicalWorkflowDefinition(
  workflowRow: SupabaseRow,
  stepRows: SupabaseRow[],
): WorkflowDefinition {
  return {
    id: String(workflowRow.id),
    name: String(workflowRow.name),
    description: String(workflowRow.description ?? ""),
    version: 1,
    status: "active",
    steps: stepRows.map((stepRow) => ({
      key: String(stepRow.step_type),
      name: String((stepRow.step_definitions as any)?.name ?? stepRow.step_type ?? ""),
      type: workflowDefinitionStepType(
        stepRow.step_type,
        (stepRow as any).requires_approval,
      ),
      outputType: undefined,
      subagent: (stepRow.step_definitions as any)?.subagent ?? null,
    })),
  };
}

function mapWorkflowRunStatus(status: string): WorkflowRun["status"] {
  const s = String(status).toUpperCase();
  if (s === "PENDING") return "draft";
  if (s === "RUNNING") return "running";
  if (s === "DONE") return "completed";
  if (s === "FAILED") return "failed";
  if (s === "CANCELED") return "rejected";
  return status.toLowerCase() as WorkflowRun["status"];
}

function mapWorkflowStepStatus(status: string): WorkflowStep["status"] {
  const s = String(status).toUpperCase();
  if (s === "PENDING") return "pending";
  if (s === "RUNNING") return "running";
  if (s === "WAITING_USER_APPROVAL") return "waiting_approval";
  if (s === "DONE") return "completed";
  if (s === "FAILED") return "failed";
  if (s === "SKIPPED") return "rejected";
  return status.toLowerCase() as WorkflowStep["status"];
}

function mapWorkflowRunStatusToCanonical(status?: WorkflowRun["status"]): string | undefined {
  if (!status) return undefined;
  if (status === "draft") return "PENDING";
  if (status === "running") return "RUNNING";
  if (status === "completed") return "DONE";
  if (status === "failed") return "FAILED";
  if (status === "rejected") return "CANCELED";
  return (status as string).toUpperCase();
}

function mapWorkflowStepStatusToCanonical(status?: WorkflowStep["status"]): string | undefined {
  if (!status) return undefined;
  if (status === "pending") return "PENDING";
  if (status === "running") return "RUNNING";
  if (status === "waiting_approval") return "WAITING_USER_APPROVAL";
  if (status === "completed") return "DONE";
  if (status === "failed") return "FAILED";
  if (status === "rejected") return "SKIPPED";
  return (status as string).toUpperCase();
}


function mapWorkflowRun(row: SupabaseRow): WorkflowRun {
  const selected = Array.isArray(row.selected_context_source_ids)
    ? row.selected_context_source_ids.map(String)
    : [];

  return {
    id: String(row.id),
    workflowDefinitionId: String(row.workflow_id || ""),
    projectId: String(row.project_id),
    status: mapWorkflowRunStatus(String(row.status || "draft")),
    currentStepKey: null,
    selectedContextSourceIds: selected,
    startedBy: String(row.started_by || ""),
    startedAt: String(row.started_at || ""),
    completedAt: row.finished_at ? String(row.finished_at) : null,
    errorSummary: row.error_message ? String(row.error_message) : null,
  };
}

function mapWorkflowRunSession(row: SupabaseRow): WorkflowRunSession {
  return {
    id: String(row.id),
    workflowRunId: String(row.workflow_run_id),
    provider: String(row.provider),
    model: String(row.model),
    transportType: String(row.transport_type),
    providerSessionId: row.provider_session_id ? String(row.provider_session_id) : null,
    processKey: row.process_key ? String(row.process_key) : null,
    processPid: row.process_pid ? Number(row.process_pid) : null,
    status: String(row.status),
    metadataJson: row.metadata_json ? (row.metadata_json as Record<string, any>) : null,
    startedAt: String(row.started_at),
    completedAt: row.completed_at ? String(row.completed_at) : null,
  };
}

function mapWorkflowStep(row: SupabaseRow): WorkflowStep {
  const rawKey = String(row.step_key || row.step_type || "").toLowerCase();
  const nestedWorkflowStep = row.workflow_steps as Record<string, unknown> | null | undefined;
  const requiresApproval =
    Boolean(row.requires_approval) || Boolean(nestedWorkflowStep?.requires_approval);
  const stepTypeVal =
    row.step_type === "tool" || row.step_type === "ai_mock" || row.step_type === "approval"
      ? (row.step_type as WorkflowStep["stepType"])
      : workflowDefinitionStepType(rawKey, requiresApproval);

  return {
    id: String(row.id),
    workflowRunId: String(row.workflow_run_id),
    stepKey: String(row.step_key || row.step_type || ""),
    stepName: String(row.step_name || (row.step_definitions as any)?.name || row.step_type || ""),
    stepType: stepTypeVal,
    status: mapWorkflowStepStatus(String(row.status || "pending")),
    sequenceIndex: Number(row.execution_order_index || 0),
    outputId: row.artifact_run_id
      ? String(row.artifact_run_id)
      : row.artifact_id
        ? String(row.artifact_id)
        : null,
    startedAt: row.started_at ? String(row.started_at) : null,
    completedAt: row.finished_at ? String(row.finished_at) : null,
    errorMessage: row.error_message ? String(row.error_message) : null,
  };
}


function mapOutput(row: SupabaseRow): AiOutput {
  return {
    id: String(row.id),
    workflowRunId: String(row.workflow_run_id),
    workflowStepId: String(row.workflow_step_id),
    projectId: String(row.project_id),
    outputType: row.output_type as AiOutput["outputType"],
    version: Number(row.version),
    title: String(row.title),
    contentMarkdown: String(row.content_markdown),
    isApproved: Boolean(row.is_approved),
    createdAt: String(row.created_at),
  };
}

function approvalDecisionRow(decision: ApprovalDecision) {
  return {
    workflow_run_step_id: decision.workflowStepId,
    log_level: "info",
    message: `${APPROVAL_DECISION_LOG_PREFIX}${JSON.stringify({
      id: decision.id,
      approvalId: decision.approvalId,
      workflowRunId: decision.workflowRunId,
      workflowStepId: decision.workflowStepId,
      aiOutputId: decision.aiOutputId,
      decision: decision.decision,
      reviewerId: decision.reviewerId,
      comment: decision.comment,
      createdAt: decision.createdAt,
    })}`,
    created_at: decision.createdAt,
  };
}

function parseApprovalDecisionLog(row: SupabaseRow): ApprovalDecision | null {
  const message = String(row.message ?? "");
  if (!message.startsWith(APPROVAL_DECISION_LOG_PREFIX)) {
    return null;
  }

  try {
    const payload = JSON.parse(message.slice(APPROVAL_DECISION_LOG_PREFIX.length)) as Record<
      string,
      unknown
    >;
    return {
      id: String(payload.id ?? row.id ?? ""),
      approvalId: String(payload.approvalId ?? ""),
      workflowRunId: String(payload.workflowRunId ?? ""),
      workflowStepId: String(payload.workflowStepId ?? row.workflow_run_step_id ?? ""),
      aiOutputId: payload.aiOutputId ? String(payload.aiOutputId) : null,
      decision: payload.decision as ApprovalDecision["decision"],
      reviewerId: payload.reviewerId ? String(payload.reviewerId) : null,
      comment: payload.comment ? String(payload.comment) : null,
      createdAt: String(payload.createdAt ?? row.created_at ?? ""),
    };
  } catch {
    return null;
  }
}

function buildApprovalId(stepId: string) {
  return `approval_${stepId}`;
}

function extractApprovalStepId(approvalId: string) {
  return approvalId.startsWith("approval_") ? approvalId.slice("approval_".length) : approvalId;
}

function mapArtifactOutput(row: SupabaseRow): AiOutput {
  return {
    id: String(row.id),
    workflowRunId: String(row.workflow_run_id),
    workflowStepId: row.workflow_run_step_id ? String(row.workflow_run_step_id) : String(row.id),
    projectId: String(row.project_id ?? ""),
    outputType: "document" as AiOutput["outputType"],
    version: 1,
    title: String(row.title || row.artifact_definition_key || "Output"),
    contentMarkdown: String(row.content_markdown ?? ""),
    isApproved: Boolean(row.is_approved ?? false),
    createdAt: String(row.created_at || ""),
  };
}

function mapLog(row: SupabaseRow): AiCallLog {
  return {
    id: String(row.id),
    workflowRunId: String(row.workflow_run_id),
    workflowStepId: String(row.workflow_step_id),
    provider: String(row.provider),
    model: String(row.model),
    inputTokens: Number(row.input_tokens),
    outputTokens: Number(row.output_tokens),
    costEstimate: Number(row.cost_estimate),
    latencyMs: Number(row.latency_ms),
    status: row.status as AiCallLog["status"],
    createdAt: String(row.created_at),
  };
}

class SupabaseGatewayBundle
  implements
    ProjectGateway,
    ContextSourceGateway,
    WorkflowGateway,
    TeamGateway,
    IntegrationGateway
{
  constructor(private readonly supabase: SupabaseClient) {}

  async listProjects() {
    const { data, error } = await this.supabase
      .from("projects")
      .select("*")
      .order("created_at", { ascending: false });
    assertNoError(error, "Unable to list projects.");
    return (data ?? []).map(mapProject);
  }

  async getProjectById(projectId: string) {
    const { data, error } = await this.supabase
      .from("projects")
      .select("*")
      .eq("id", projectId)
      .maybeSingle();
    assertNoError(error, "Unable to load project.");
    return data ? mapProject(data) : null;
  }

  async updateProject(projectId: string, patch: Partial<UpdateProjectPayload>) {
    const { data, error } = await this.supabase
      .from("projects")
      .update(buildProjectUpdatePayload(patch))
      .eq("id", projectId)
      .select("*")
      .single();
    assertNoError(error, "Unable to update project.");
    return mapProject(assertRow(data, "Project update returned no row."));
  }

  async deleteProject(projectId: string) {
    const { error } = await this.supabase.from("projects").delete().eq("id", projectId);
    assertNoError(error, "Unable to delete project.");
  }

  async listTeamsByProject(projectId: string) {
    const { data, error } = await this.supabase
      .from("project_teams")
      .select("team_id, teams(*)")
      .eq("project_id", projectId);
    assertNoError(error, "Unable to list project teams.");
    return (data ?? [])
      .map((row) => {
        const typed = row as { teams?: SupabaseRow | SupabaseRow[] | null };
        if (Array.isArray(typed.teams)) {
          return typed.teams[0] ?? null;
        }
        return typed.teams ?? null;
      })
      .filter((row): row is SupabaseRow => Boolean(row))
      .map(mapTeam);
  }

  async listProjectWorkspaceBindings(projectId: string) {
    const { data, error } = await this.supabase
      .from("project_workspace_bindings")
      .select("*")
      .eq("project_id", projectId)
      .order("created_at", { ascending: true });
    assertNoError(error, "Unable to list project directory bindings.");
    return (data ?? []).map(mapProjectWorkspaceBinding);
  }

  async listIntegrationsByProject(projectId: string) {
    const { data, error } = await this.supabase
      .from("integrations")
      .select("*")
      .eq("project_id", projectId)
      .order("created_at", { ascending: false });
    assertNoError(error, "Unable to list project integrations.");
    return (data ?? []).map(mapIntegration);
  }

  async listAllIntegrations() {
    const { data, error } = await this.supabase
      .from("integrations")
      .select("*")
      .order("updated_at", { ascending: false });
    assertNoError(error, "Unable to list MCP instances.");
    return (data ?? []).map(mapIntegration);
  }

  async listLinkedIntegrationsByProject(projectId: string) {
    const { data, error } = await this.supabase
      .from("project_mcp_links")
      .select("integration_id, integrations(*)")
      .eq("project_id", projectId)
      .order("updated_at", { ascending: false });
    assertNoError(error, "Unable to list linked project MCP instances.");
    return (data ?? [])
      .map((row) => {
        const typed = row as { integrations?: SupabaseRow | SupabaseRow[] | null };
        if (Array.isArray(typed.integrations)) {
          return typed.integrations[0] ?? null;
        }
        return typed.integrations ?? null;
      })
      .filter((row): row is SupabaseRow => Boolean(row))
      .map(mapIntegration);
  }

  async createProject(payload: CreateProjectPayload) {
    const primaryPath = payload.directoryPath.trim();
    const defaultModel = payload.defaultModel?.trim() || "gpt-5.4";
    const defaultReasoningEffort = payload.defaultReasoningEffort?.trim() || "medium";
    const projectId = createUuid();
    const legacyProjectId = createId("project");
    const { data, error } = await this.supabase
      .from("projects")
      .insert({
        id: projectId,
        legacy_id: legacyProjectId,
        name: payload.name,
        description: payload.description,
        platform: payload.platform,
        repository_url: payload.repositoryUrl,
        directory_path: primaryPath,
        default_provider: resolveProviderKeyFromModel(defaultModel),
        default_model: defaultModel,
        default_reasoning_effort: defaultReasoningEffort,
        created_by: "supabase-admin",
      })
      .select("*")
      .single();
    assertNoError(error, "Unable to create project.");
    const project = mapProject(assertRow(data, "Project insert returned no row."));

    if (primaryPath) {
      await this.createProjectWorkspaceBinding(project.id, {
        localPath: primaryPath,
        label: "Primary",
      });
    }

    return project;
  }

  async createProjectWorkspaceBinding(
    projectId: string,
    payload: CreateProjectWorkspaceBindingPayload,
  ) {
    const localPath = payload.localPath.trim();
    const label = payload.label?.trim() || null;
    const { data, error } = await this.supabase
      .from("project_workspace_bindings")
      .insert({
        id: createUuid(),
        project_id: projectId,
        local_path: localPath,
        label,
      })
      .select("*")
      .single();
    assertNoError(error, "Unable to create project directory binding.");
    return mapProjectWorkspaceBinding(assertRow(data, "Project binding insert returned no row."));
  }

  async updateProjectWorkspaceBinding(
    bindingId: string,
    patch: Partial<UpdateProjectWorkspaceBindingPayload>,
  ) {
    const updates: Record<string, unknown> = { updated_at: new Date().toISOString() };
    if (patch.localPath !== undefined) {
      updates.local_path = patch.localPath.trim();
    }
    if (patch.label !== undefined) {
      updates.label = patch.label?.trim() || null;
    }

    const { data, error } = await this.supabase
      .from("project_workspace_bindings")
      .update(updates)
      .eq("id", bindingId)
      .select("*")
      .single();
    assertNoError(error, "Unable to update project directory binding.");
    return mapProjectWorkspaceBinding(assertRow(data, "Project binding update returned no row."));
  }

  async deleteProjectWorkspaceBinding(bindingId: string) {
    const { error } = await this.supabase
      .from("project_workspace_bindings")
      .delete()
      .eq("id", bindingId);
    assertNoError(error, "Unable to delete project directory binding.");
  }

  async createIntegration(payload: CreateIntegrationPayload) {
    const { data, error } = await this.supabase
      .from("integrations")
      .insert({
        id: createUuid(),
        project_id: payload.projectId,
        type: payload.type,
        label: payload.label,
        config_encrypted: payload.configEncrypted,
        status: payload.status ?? "pending",
        mcp_type_enabled: payload.mcpTypeEnabled ?? false,
        last_error: null,
      })
      .select("*")
      .single();
    assertNoError(error, "Unable to create integration.");
    const integration = mapIntegration(assertRow(data, "Integration insert returned no row."));
    await this.linkIntegrationToProject(payload.projectId, integration.id);
    return integration;
  }

  async updateIntegration(integrationId: string, patch: Partial<UpdateIntegrationPayload>) {
    const { data, error } = await this.supabase
      .from("integrations")
      .update({
        type: patch.type,
        label: patch.label,
        config_encrypted: patch.configEncrypted,
        status: patch.status,
        mcp_type_enabled: patch.mcpTypeEnabled,
        last_synced_at: patch.lastSyncedAt,
        last_error: patch.lastError,
        updated_at: new Date().toISOString(),
      })
      .eq("id", integrationId)
      .select("*")
      .single();
    assertNoError(error, "Unable to update integration.");
    return mapIntegration(assertRow(data, "Integration update returned no row."));
  }

  async deleteIntegration(integrationId: string) {
    const { error } = await this.supabase
      .from("integrations")
      .delete()
      .eq("id", integrationId);
    assertNoError(error, "Unable to delete integration.");
  }

  async linkIntegrationToProject(projectId: string, integrationId: string) {
    const { data: integrationData, error: integrationError } = await this.supabase
      .from("integrations")
      .select("id, type")
      .eq("id", integrationId)
      .single();
    assertNoError(integrationError, "Unable to load MCP instance before linking.");
    const integrationRow = assertRow(integrationData, "MCP instance row not found.");
    const { error } = await this.supabase
      .from("project_mcp_links")
      .upsert(
        {
          project_id: projectId,
          integration_id: integrationId,
          type: integrationRow.type,
          updated_at: new Date().toISOString(),
        },
        { onConflict: "project_id,type" },
      );
    assertNoError(error, "Unable to link MCP instance to project.");
  }

  async unlinkIntegrationFromProject(projectId: string, integrationId: string) {
    const { error } = await this.supabase
      .from("project_mcp_links")
      .delete()
      .eq("project_id", projectId)
      .eq("integration_id", integrationId);
    assertNoError(error, "Unable to unlink MCP instance from project.");
  }

  async listTeams() {
    const { data, error } = await this.supabase
      .from("teams")
      .select("*")
      .order("created_at", { ascending: false });
    assertNoError(error, "Unable to list teams.");
    return (data ?? []).map(mapTeam);
  }

  async getTeamById(teamId: string) {
    const { data, error } = await this.supabase
      .from("teams")
      .select("*")
      .eq("id", teamId)
      .maybeSingle();
    assertNoError(error, "Unable to load team.");
    return data ? mapTeam(data) : null;
  }

  async createTeam(name: string) {
    const { data, error } = await this.supabase
      .from("teams")
      .insert({ id: createUuid(), name })
      .select("*")
      .single();
    assertNoError(error, "Unable to create team.");
    return mapTeam(assertRow(data, "Team insert returned no row."));
  }

  async updateTeam(teamId: string, name: string) {
    const { data, error } = await this.supabase
      .from("teams")
      .update({ name })
      .eq("id", teamId)
      .select("*")
      .single();
    assertNoError(error, "Unable to update team.");
    return mapTeam(assertRow(data, "Team update returned no row."));
  }

  async deleteTeam(teamId: string) {
    const { error } = await this.supabase.from("teams").delete().eq("id", teamId);
    assertNoError(error, "Unable to delete team.");
  }

  async listMembersByTeam(teamId: string) {
    const { data, error } = await this.supabase
      .from("team_members")
      .select("*")
      .eq("team_id", teamId)
      .order("created_at", { ascending: false });
    assertNoError(error, "Unable to list team members.");
    return (data ?? []).map(mapTeamMember);
  }

  async addMember(member: Omit<TeamMember, "id" | "createdAt" | "updatedAt">) {
    const { data, error } = await this.supabase
      .from("team_members")
      .insert({
        id: createUuid(),
        team_id: member.teamId,
        name: member.name,
        email: member.email,
        jira_account_id: member.jiraAccountId,
        role: member.role,
        level_label: member.levelLabel,
        skill_tags: member.skillTags,
        weekly_capacity_hours: member.weeklyCapacityHours,
      })
      .select("*")
      .single();
    assertNoError(error, "Unable to add team member.");
    return mapTeamMember(assertRow(data, "Team member insert returned no row."));
  }

  async updateMember(memberId: string, patch: Partial<TeamMember>) {
    const { data, error } = await this.supabase
      .from("team_members")
      .update({
        team_id: patch.teamId,
        name: patch.name,
        email: patch.email,
        jira_account_id: patch.jiraAccountId,
        role: patch.role,
        level_label: patch.levelLabel,
        skill_tags: patch.skillTags,
        weekly_capacity_hours: patch.weeklyCapacityHours,
      })
      .eq("id", memberId)
      .select("*")
      .single();
    assertNoError(error, "Unable to update team member.");
    return mapTeamMember(assertRow(data, "Team member update returned no row."));
  }

  async removeMember(memberId: string) {
    const { error } = await this.supabase.from("team_members").delete().eq("id", memberId);
    assertNoError(error, "Unable to remove team member.");
  }

  async linkTeamToProject(projectId: string, teamId: string) {
    const { data: projectData, error: projectError } = await this.supabase
      .from("projects")
      .select("legacy_id")
      .eq("id", projectId)
      .single();
    assertNoError(projectError, "Unable to load project before linking team.");
    const projectRow = assertRow(projectData, "Project row not found before team link.");

    const { error } = await this.supabase
      .from("project_teams")
      .insert({
        id: createUuid(),
        project_id: projectId,
        legacy_project_id: projectRow.legacy_id,
        team_id: teamId,
      });
    assertNoError(error, "Unable to link team to project.");
  }

  async unlinkTeamFromProject(projectId: string, teamId: string) {
    const { error } = await this.supabase
      .from("project_teams")
      .delete()
      .eq("project_id", projectId)
      .eq("team_id", teamId);
    assertNoError(error, "Unable to unlink team from project.");
  }

  async listContextSources() {
    const { data, error } = await this.supabase
      .from("context_sources")
      .select("*")
      .is("archived_at", null)
      .order("created_at", { ascending: false });
    assertNoError(error, "Unable to list context sources.");
    return (data ?? []).map(mapContextSource);
  }

  async listContextSourcesByProject(projectId: string) {
    const { data, error } = await this.supabase
      .from("context_sources")
      .select("*")
      .eq("project_id", projectId)
      .is("archived_at", null)
      .order("created_at", { ascending: false });
    assertNoError(error, "Unable to list project context sources.");
    return (data ?? []).map(mapContextSource);
  }

  async getContextSourceById(contextSourceId: string) {
    const { data, error } = await this.supabase
      .from("context_sources")
      .select("*")
      .eq("id", contextSourceId)
      .is("archived_at", null)
      .maybeSingle();
    assertNoError(error, "Unable to load context source.");
    return data ? mapContextSource(data) : null;
  }

  async createContextSource(payload: CreateContextSourcePayload) {
    const { data, error } = await this.supabase
      .from("context_sources")
      .insert({
        id: createId("context"),
        project_id: payload.projectId,
        type: payload.type,
        title: payload.title,
        raw_content: payload.rawContent,
        summarized_content: null,
        created_by: "supabase-admin",
      })
      .select("*")
      .single();
    assertNoError(error, "Unable to create context source.");
    return mapContextSource(assertRow(data, "Context source insert returned no row."));
  }

  async updateContextSource(payload: UpdateContextSourcePayload) {
    const { data, error } = await this.supabase
      .from("context_sources")
      .update({
        title: payload.title,
        type: payload.type,
        raw_content: payload.rawContent,
        summarized_content: payload.summarizedContent ?? null,
      })
      .eq("id", payload.contextSourceId)
      .select("*")
      .single();
    assertNoError(error, "Unable to update context source.");
    return mapContextSource(assertRow(data, "Context source update returned no row."));
  }

  async deleteContextSource(contextSourceId: string) {
    const { error } = await this.supabase
      .from("context_sources")
      .update({ archived_at: new Date().toISOString() })
      .eq("id", contextSourceId);
    assertNoError(error, "Unable to archive context source.");
  }

  async listWorkflowDefinitions() {
    const [{ data: workflows, error: workflowsError }, { data: steps, error: stepsError }] =
      await Promise.all([
        this.supabase.from("workflows").select("*").order("created_at", { ascending: false }),
        this.supabase
          .from("workflow_steps")
          .select("workflow_id, step_type, order_index, requires_approval, step_definitions(name, subagent)")
          .order("order_index", { ascending: true }),
      ]);

    assertNoError(workflowsError, "Unable to list workflow definitions.");
    assertNoError(stepsError, "Unable to list workflow definition steps.");

    const stepsByWorkflowId = new Map<string, SupabaseRow[]>();
    for (const row of steps ?? []) {
      const workflowId = String((row as any).workflow_id);
      const current = stepsByWorkflowId.get(workflowId) ?? [];
      current.push(row as SupabaseRow);
      stepsByWorkflowId.set(workflowId, current);
    }

    return (workflows ?? []).map((workflowRow) =>
      mapCanonicalWorkflowDefinition(
        workflowRow as SupabaseRow,
        stepsByWorkflowId.get(String((workflowRow as any).id)) ?? [],
      ),
    );
  }

  async getWorkflowDefinitionById(workflowDefinitionId: string) {
    const { data: workflowRow, error: workflowError } = await this.supabase
      .from("workflows")
      .select("*")
      .eq("id", workflowDefinitionId)
      .maybeSingle();
    assertNoError(workflowError, "Unable to load workflow definition.");
    if (!workflowRow) {
      return null;
    }

    const { data: stepRows, error: stepsError } = await this.supabase
      .from("workflow_steps")
      .select("workflow_id, step_type, order_index, requires_approval, step_definitions(name, subagent)")
      .eq("workflow_id", workflowDefinitionId)
      .order("order_index", { ascending: true });
    assertNoError(stepsError, "Unable to load workflow definition steps.");

    return mapCanonicalWorkflowDefinition(
      workflowRow as SupabaseRow,
      (stepRows ?? []) as SupabaseRow[],
    );
  }

  async listWorkflowRuns() {
    const { data, error } = await this.supabase
      .from("workflow_runs")
      .select("*")
      .order("started_at", { ascending: false });
    assertNoError(error, "Unable to list workflow runs.");
    return (data ?? []).map(mapWorkflowRun);
  }

  async getWorkflowRunById(runId: string) {
    const { data, error } = await this.supabase
      .from("workflow_runs")
      .select("*")
      .eq("id", runId)
      .maybeSingle();
    assertNoError(error, "Unable to load workflow run.");
    return data ? mapWorkflowRun(data) : null;
  }

  async deleteWorkflowRuns(runIds: string[]) {
    const ids = [...new Set(runIds)].filter((runId) => runId.trim().length > 0);
    if (ids.length === 0) {
      return;
    }

    const { error } = await this.supabase.from("workflow_runs").delete().in("id", ids);
    assertNoError(error, "Unable to delete workflow runs.");
  }

  async getWorkflowRunDetail(runId: string) {
    const run = await this.getWorkflowRunById(runId);

    if (!run) {
      return null;
    }

    const [steps, outputs, logs, approvalDecisions, contexts, project, definition, sessions] =
      await Promise.all([
        this.listStepsByRun(run.id),
        this.listOutputsByRun(run.id),
        this.listLogsByRun(run.id),
        this.listApprovalDecisionsByRun(run.id),
        this.listContextSourcesByIds(run.selectedContextSourceIds),
        this.getProjectById(run.projectId),
        this.getWorkflowDefinitionById(run.workflowDefinitionId),
        this.listSessionsByRun(run.id),
      ]);

    const approvals = this.buildApprovals(steps, outputs, approvalDecisions);

    return {
      run,
      steps,
      outputs,
      approvals,
      logs,
      approvalDecisions,
      selectedContextSources: contexts,
      project,
      definition,
      sessions,
    };
  }

  async createWorkflowRun(payload: StartWorkflowRunPayload, definition: WorkflowDefinition) {
    const project = await this.getProjectById(payload.projectId);

    if (!project) {
      throw new Error("Project not found.");
    }

    const runId = createUuid();

    const { data: runData, error } = await this.supabase
      .from("workflow_runs")
      .insert({
        id: runId,
        workflow_id: definition.id,
        project_id: project.id,
        status: "RUNNING",
        started_by: "supabase-admin",
      })
      .select("*")
      .single();
    assertNoError(error, "Unable to create workflow run.");

    const { data: definitionSteps, error: definitionStepsError } = await this.supabase
      .from("workflow_steps")
      .select("id, step_type, order_index")
      .eq("workflow_id", definition.id)
      .eq("is_enabled", true)
      .order("order_index", { ascending: true });
    assertNoError(definitionStepsError, "Unable to load workflow definition steps.");

    const stepRows = (definitionSteps ?? []).map((step, index) => ({
      id: createUuid(),
      workflow_run_id: runId,
      workflow_step_id: String((step as any).id),
      step_type: String((step as any).step_type),
      status: "PENDING",
      execution_order_index: index,
    }));

    if (stepRows.length > 0) {
      const { error: stepError } = await this.supabase
        .from("workflow_run_steps")
        .insert(stepRows);
      assertNoError(stepError, "Unable to create workflow run steps.");
    }

    return mapWorkflowRun(assertRow(runData, "Workflow run insert returned no row."));
  }

  async updateWorkflowRun(runId: string, patch: Partial<WorkflowRun>) {
    const { data: runData, error } = await this.supabase
      .from("workflow_runs")
      .update({
        status: mapWorkflowRunStatusToCanonical(patch.status),
        finished_at: patch.completedAt,
        error_message: patch.errorSummary,
      })
      .eq("id", runId)
      .select("*")
      .single();
    assertNoError(error, "Unable to update workflow run.");
    return mapWorkflowRun(assertRow(runData, "Workflow run update returned no row."));
  }

  async updateWorkflowStep(stepId: string, patch: Partial<WorkflowStep>) {
    const { data: stepData, error } = await this.supabase
      .from("workflow_run_steps")
      .update({
        status: mapWorkflowStepStatusToCanonical(patch.status),
        artifact_run_id: patch.outputId,
        started_at: patch.startedAt,
        finished_at: patch.completedAt,
        error_message: patch.errorMessage,
      })
      .eq("id", stepId)
      .select("*, step_definitions(name), workflow_steps(requires_approval)")
      .single();
    assertNoError(error, "Unable to update workflow step.");
    return mapWorkflowStep(assertRow(stepData, "Workflow step update returned no row."));
  }

  async createOutput(output: AiOutput) {
    return output;
  }

  async createApproval(approval: Approval) {
    return approval;
  }

  async getApprovalById(approvalId: string) {
    const stepId = extractApprovalStepId(approvalId);
    const step = await this.getStepById(stepId);
    if (!step) {
      return null;
    }

    const [outputs, decisions] = await Promise.all([
      this.listOutputsByRun(step.workflowRunId),
      this.listApprovalDecisionsByRun(step.workflowRunId),
    ]);
    return this.buildApproval(step, outputs, decisions);
  }

  async listPendingApprovalDetails() {
    const { data: stepRows, error } = await this.supabase
      .from("workflow_run_steps")
      .select("*, step_definitions(name), workflow_steps(requires_approval)")
      .eq("status", "WAITING_USER_APPROVAL")
      .order("started_at", { ascending: false });
    assertNoError(error, "Unable to list pending approvals.");

    const steps = (stepRows ?? []).map(mapWorkflowStep);
    const details = await Promise.all(
      steps.map(async (step) => {
        const run = await this.getWorkflowRunById(step.workflowRunId);
        if (!run) {
          return null;
        }

        const [outputs, decisions, project] = await Promise.all([
          this.listOutputsByRun(run.id),
          this.listApprovalDecisionsByRun(run.id),
          this.getProjectById(run.projectId),
        ]);
        const approval = this.buildApproval(step, outputs, decisions);
        if (!approval || approval.status !== "pending") {
          return null;
        }

        return {
          approval,
          run,
          step,
          output: approval.aiOutputId ? outputs.find((item) => item.id === approval.aiOutputId) ?? null : null,
          project,
        };
      }),
    );

    return details.filter((detail) => detail !== null);
  }

  async updateApproval(approvalId: string, patch: Partial<Approval>) {
    const approval = await this.getApprovalById(approvalId);
    if (!approval) {
      throw new Error("Approval not found.");
    }

    return {
      ...approval,
      ...patch,
      id: approval.id,
      workflowRunId: approval.workflowRunId,
      workflowStepId: approval.workflowStepId,
      aiOutputId: patch.aiOutputId ?? approval.aiOutputId,
      createdAt: approval.createdAt,
    };
  }

  async createApprovalDecision(decision: ApprovalDecision) {
    const { data, error } = await this.supabase
      .from("workflow_run_logs")
      .insert(approvalDecisionRow(decision))
      .select("id, workflow_run_step_id, message, created_at")
      .single();
    assertNoError(error, "Unable to create approval decision log.");
    return (
      parseApprovalDecisionLog(assertRow(data as SupabaseRow, "Approval decision insert returned no row.")) ??
      decision
    );
  }

  async listApprovalDecisionsByRun(runId: string) {
    const { data: steps, error: stepsError } = await this.supabase
      .from("workflow_run_steps")
      .select("id")
      .eq("workflow_run_id", runId);
    assertNoError(stepsError, "Unable to load run step ids for approval decisions.");

    const stepIds = (steps ?? []).map((row) => String((row as any).id));
    if (stepIds.length === 0) {
      return [];
    }

    const { data, error } = await this.supabase
      .from("workflow_run_logs")
      .select("id, workflow_run_step_id, message, created_at")
      .in("workflow_run_step_id", stepIds)
      .order("created_at", { ascending: false });
    assertNoError(error, "Unable to list approval decisions.");
    return (data ?? [])
      .map((row) => parseApprovalDecisionLog(row as SupabaseRow))
      .filter((row): row is ApprovalDecision => row !== null);
  }

  async listOutputs(filters?: ListOutputsFilters) {
    let query = this.supabase
      .from("artifact_runs")
      .select("id, project_id, workflow_run_id, workflow_run_step_id, title, created_at, artifact_definition_key")
      .order("created_at", { ascending: false });

    if (filters?.projectId) {
      query = query.eq("project_id", filters.projectId);
    }

    if (filters?.workflowRunId) {
      query = query.eq("workflow_run_id", filters.workflowRunId);
    }

    const { data, error } = await query;
    assertNoError(error, "Unable to list workflow outputs.");
    return (data ?? []).map((row) => mapArtifactOutput(row as SupabaseRow));
  }

  async getOutputDetail(outputId: string) {
    const output = await this.getOutputById(outputId);

    if (!output) {
      return null;
    }

    const [run, steps, project, approvals, approvalDecisions, versions] =
      await Promise.all([
      this.getWorkflowRunById(output.workflowRunId),
      this.listStepsByRun(output.workflowRunId),
      this.getProjectById(output.projectId),
        this.listApprovalsByOutput(output.id),
        this.listApprovalDecisionsByOutput(output.id),
        this.listOutputVersions(output),
      ]);

    return {
      output,
      run,
      step: steps.find((step) => step.id === output.workflowStepId) ?? null,
      project,
      approvals,
      approvalDecisions,
      versions,
    };
  }

  async createLog(log: AiCallLog) {
    const { data, error } = await this.supabase
      .from("ai_call_logs")
      .insert({
        id: log.id,
        workflow_run_id: log.workflowRunId,
        workflow_step_id: log.workflowStepId,
        provider: log.provider,
        model: log.model,
        input_tokens: log.inputTokens,
        output_tokens: log.outputTokens,
        cost_estimate: log.costEstimate,
        latency_ms: log.latencyMs,
        status: log.status,
      })
      .select("*")
      .single();
    assertNoError(error, "Unable to create AI call log.");
    return mapLog(assertRow(data, "AI call log insert returned no row."));
  }

  async listLogs(filters?: { status?: AiCallLog["status"]; provider?: string }) {
    let query = this.supabase
      .from("ai_call_logs")
      .select("*")
      .order("created_at", { ascending: false });

    if (filters?.status) {
      query = query.eq("status", filters.status);
    }

    if (filters?.provider) {
      query = query.eq("provider", filters.provider);
    }

    const { data, error } = await query;
    assertNoError(error, "Unable to list AI call logs.");
    return (data ?? []).map(mapLog);
  }

  async getLogSummary(filters?: { status?: AiCallLog["status"]; provider?: string }) {
    const logs = await this.listLogs(filters);
    const totalLatency = logs.reduce((sum, log) => sum + log.latencyMs, 0);

    return {
      totalCalls: logs.length,
      totalInputTokens: logs.reduce((sum, log) => sum + log.inputTokens, 0),
      totalOutputTokens: logs.reduce((sum, log) => sum + log.outputTokens, 0),
      totalCostEstimate: logs.reduce((sum, log) => sum + log.costEstimate, 0),
      averageLatencyMs: logs.length === 0 ? 0 : Math.round(totalLatency / logs.length),
      failedCalls: logs.filter((log) => log.status === "failed").length,
    };
  }

  private async listStepsByRun(runId: string) {
    const { data, error } = await this.supabase
      .from("workflow_run_steps")
      .select("*, step_definitions(name), workflow_steps(requires_approval)")
      .eq("workflow_run_id", runId)
      .order("execution_order_index", { ascending: true });
    assertNoError(error, "Unable to list workflow steps.");
    return (data ?? []).map(mapWorkflowStep);
  }

  private async listOutputsByRun(runId: string): Promise<AiOutput[]> {
    const { data: artifactData, error: artifactError } = await this.supabase
      .from("artifact_runs")
      .select("id, project_id, workflow_run_id, workflow_run_step_id, title, created_at, artifact_definition_key")
      .eq("workflow_run_id", runId)
      .order("created_at", { ascending: true });

    if (artifactError) {
      console.warn("Unable to list artifact_runs outputs:", artifactError.message);
      return [];
    }

    return (artifactData ?? []).map((row) => mapArtifactOutput(row as SupabaseRow));
  }

  private async listApprovalsByRun(runId: string) {
    const [steps, outputs, decisions] = await Promise.all([
      this.listStepsByRun(runId),
      this.listOutputsByRun(runId),
      this.listApprovalDecisionsByRun(runId),
    ]);
    return this.buildApprovals(steps, outputs, decisions);
  }

  private async listLogsByRun(runId: string) {
    const { data, error } = await this.supabase
      .from("ai_call_logs")
      .select("*")
      .eq("workflow_run_id", runId)
      .order("created_at", { ascending: true });
    assertNoError(error, "Unable to list run logs.");
    return (data ?? []).map(mapLog);
  }

  private async listSessionsByRun(runId: string) {
    const { data, error } = await this.supabase
      .from("workflow_run_sessions")
      .select("*")
      .eq("workflow_run_id", runId)
      .order("started_at", { ascending: true });
    assertNoError(error, "Unable to list run sessions.");
    return (data ?? []).map(mapWorkflowRunSession);
  }

  private async listContextSourcesByIds(contextSourceIds: string[]) {
    if (contextSourceIds.length === 0) {
      return [];
    }

    const { data, error } = await this.supabase
      .from("context_sources")
      .select("*")
      .in("id", contextSourceIds)
      .is("archived_at", null);
    assertNoError(error, "Unable to list selected context sources.");
    return (data ?? []).map(mapContextSource);
  }

  private async getOutputById(outputId: string) {
    const { data, error } = await this.supabase
      .from("artifact_runs")
      .select("id, project_id, workflow_run_id, workflow_run_step_id, title, created_at, artifact_definition_key")
      .eq("id", outputId)
      .maybeSingle();
    assertNoError(error, "Unable to load workflow output.");
    return data ? mapArtifactOutput(data as SupabaseRow) : null;
  }

  private async listApprovalsByOutput(outputId: string) {
    const output = await this.getOutputById(outputId);
    if (!output) {
      return [];
    }

    const [steps, outputs, decisions] = await Promise.all([
      this.listStepsByRun(output.workflowRunId),
      this.listOutputsByRun(output.workflowRunId),
      this.listApprovalDecisionsByRun(output.workflowRunId),
    ]);
    return this.buildApprovals(steps, outputs, decisions).filter(
      (approval) => approval.aiOutputId === outputId,
    );
  }

  private async listApprovalDecisionsByOutput(outputId: string) {
    const output = await this.getOutputById(outputId);
    if (!output) {
      return [];
    }

    const decisions = await this.listApprovalDecisionsByRun(output.workflowRunId);
    return decisions.filter((decision) => decision.aiOutputId === outputId);
  }

  private async listOutputVersions(output: AiOutput) {
    const { data, error } = await this.supabase
      .from("artifact_runs")
      .select("id, project_id, workflow_run_id, workflow_run_step_id, title, created_at, artifact_definition_key")
      .eq("workflow_run_id", output.workflowRunId)
      .eq("title", output.title)
      .order("created_at", { ascending: false });
    assertNoError(error, "Unable to list output versions.");
    return (data ?? []).map((row) => mapArtifactOutput(row as SupabaseRow));
  }

  private async getStepById(stepId: string) {
    const { data, error } = await this.supabase
      .from("workflow_run_steps")
      .select("*, step_definitions(name), workflow_steps(requires_approval)")
      .eq("id", stepId)
      .maybeSingle();
    assertNoError(error, "Unable to load workflow step.");
    return data ? mapWorkflowStep(data as SupabaseRow) : null;
  }

  private buildApproval(
    step: WorkflowStep,
    outputs: AiOutput[],
    decisions: ApprovalDecision[],
  ): Approval | null {
    const latestDecision = decisions.find((decision) => decision.workflowStepId === step.id) ?? null;
    const latestOutput =
      outputs.find((output) => output.workflowStepId === step.id) ?? outputs.at(-1) ?? null;

    const derivedStatus =
      step.status === "waiting_approval"
        ? "pending"
        : latestDecision?.decision ?? (step.status === "rejected" ? "rejected" : null);

    if (step.stepType !== "approval" && !derivedStatus) {
      return null;
    }

    return {
      id: buildApprovalId(step.id),
      workflowRunId: step.workflowRunId,
      workflowStepId: step.id,
      aiOutputId: latestOutput?.id ?? null,
      status: (derivedStatus ?? "approved") as Approval["status"],
      reviewerId: latestDecision?.reviewerId ?? null,
      comment: latestDecision?.comment ?? step.errorMessage ?? null,
      decidedAt: latestDecision?.createdAt ?? step.completedAt,
      createdAt: step.startedAt ?? step.completedAt ?? new Date(0).toISOString(),
    };
  }

  private buildApprovals(
    steps: WorkflowStep[],
    outputs: AiOutput[],
    decisions: ApprovalDecision[],
  ) {
    return steps
      .map((step) => this.buildApproval(step, outputs, decisions))
      .filter((approval): approval is Approval => approval !== null);
  }
}

export function createSupabaseGatewayBundle(supabaseClient: SupabaseClient) {
  const supabaseGatewayBundle = new SupabaseGatewayBundle(supabaseClient);
  const workflowExecutor: WorkflowExecutorGateway = new MockWorkflowExecutor(
    supabaseGatewayBundle,
    (projectId) => supabaseGatewayBundle.getProjectById(projectId),
  );

  return {
    projectGateway: supabaseGatewayBundle,
    contextSourceGateway: supabaseGatewayBundle,
    workflowGateway: supabaseGatewayBundle,
    teamGateway: supabaseGatewayBundle,
    integrationGateway: supabaseGatewayBundle,
    workflowExecutor,
  };
}
