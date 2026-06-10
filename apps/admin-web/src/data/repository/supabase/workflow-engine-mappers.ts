import type {
  ArtifactDefinition,
  ArtifactRun,
  ArtifactRunReplica,
  ReasoningEffort,
  SupportedStepModel,
  StepDefinition,
  Workflow,
  WorkflowStep,
  WorkflowRun,
  WorkflowRunStep,
  WorkflowRunLog,
  WorkflowRunSession,
  StepType,
  WorkflowRunStatus,
  WorkflowStepStatus,
  SupportedModel,
} from "@/domain/model/entity/workflow-engine";
import {
  STEP_MODEL_OPTIONS,
  coerceSupportedStepModel,
  normalizeMcpAccessMode,
  normalizeStepModel,
} from "@/domain/model/entity/workflow-engine";

export interface SupabaseRow {
  [key: string]: any;
}

function toStringArray(value: unknown) {
  return Array.isArray(value) ? value.map(String).filter(Boolean) : [];
}

export function mapStepDefinition(row: SupabaseRow): StepDefinition {
  const mcps = Array.isArray(row.required_mcps)
    ? row.required_mcps.map(String)
    : [];
  const skills = Array.isArray(row.required_skills)
    ? row.required_skills.map(String)
    : [];
  const inputArtifactDefinitions = toStringArray(
    row.input_artifact_definition_keys ?? row.input_artifact_definitions,
  );
  const outputArtifactDefinitions = toStringArray(
    row.output_artifact_definition_keys ?? row.output_artifact_definitions,
  );

  return {
    stepType: row.step_type as StepType,
    name: String(row.name),
    description: String(row.description),
    promptBase: row.prompt_base ? String(row.prompt_base) : null,
    requiredMcps: mcps,
    mcpAccessMode: normalizeMcpAccessMode(row.mcp_access_mode),
    requiredSkills: skills,
    teamRole: row.team_role ? String(row.team_role) : null,
    subagent: row.subagent ? String(row.subagent) : null,
    model: coerceSupportedStepModel(
      row.model
        ? String(row.model)
      : STEP_MODEL_OPTIONS.find((option) => option.value === "gpt-5.4")?.value ??
        STEP_MODEL_OPTIONS[0].value,
      STEP_MODEL_OPTIONS.find((option) => option.value === "gpt-5.4")?.value ??
        STEP_MODEL_OPTIONS[0].value,
    ) as SupportedStepModel,
    reasoningEffort: row.reasoning_effort ? (String(row.reasoning_effort) as ReasoningEffort) : null,
    agentType: row.agent_type as "standard" | "autonomous",
    inputArtifactDefinitions,
    outputArtifactDefinitions,
    createdAt: row.created_at ? String(row.created_at) : "",
    updatedAt: row.updated_at ? String(row.updated_at) : "",
  };
}

export function mapArtifactDefinition(row: SupabaseRow): ArtifactDefinition {
  return {
    key: String(row.key ?? row.artifact_key),
    name: String(row.name),
    description: String(row.description),
    localPathTemplate: String(row.local_path_template),
    remotePathTemplate: String(row.remote_path_template ?? ""),
    defaultFileName: String(row.default_file_name ?? ""),
    createdAt: row.created_at ? String(row.created_at) : "",
    updatedAt: row.updated_at ? String(row.updated_at) : "",
  };
}

export function mapArtifactRun(row: SupabaseRow): ArtifactRun {
  const artifactDefinitionKey = row.artifact_definition_key ?? row.artifact_key;

  return {
    id: String(row.id),
    artifactDefinitionKey: artifactDefinitionKey ? String(artifactDefinitionKey) : null,
    workflowId: String(row.workflow_id),
    workflowRunId: String(row.workflow_run_id),
    workflowRunStepId: row.workflow_run_step_id ? String(row.workflow_run_step_id) : null,
    projectId: row.project_id ? String(row.project_id) : null,
    title: String(row.title),
    localPath: String(row.local_path),
    remotePath: row.remote_path ? String(row.remote_path) : "",
    remoteUrl: row.remote_url ? String(row.remote_url) : "",
    storageProvider:
      row.storage_provider === "google_drive"
        ? "google_drive"
        : row.storage_provider === "supabase"
          ? "supabase"
          : null,
    remoteObjectId: row.remote_object_id ? String(row.remote_object_id) : null,
    syncStatus: row.sync_status as ArtifactRun["syncStatus"],
    replicas: [],
    createdAt: row.created_at ? String(row.created_at) : "",
    updatedAt: row.updated_at ? String(row.updated_at) : "",
  };
}

export function mapArtifactRunReplica(row: SupabaseRow): ArtifactRunReplica {
  return {
    id: String(row.id ?? ""),
    artifactRunId: String(row.artifact_run_id),
    projectId: row.project_id ? String(row.project_id) : null,
    provider: row.provider === "google_drive" ? "google_drive" : "supabase",
    storageScopeKey: row.storage_scope_key ? String(row.storage_scope_key) : null,
    remotePath: String(row.remote_path ?? ""),
    remoteObjectId: row.remote_object_id ? String(row.remote_object_id) : null,
    syncStatus:
      row.sync_status === "syncing"
        ? "syncing"
        : row.sync_status === "synced"
          ? "synced"
          : row.sync_status === "failed"
            ? "failed"
            : "queued",
    checksum: row.checksum ? String(row.checksum) : null,
    lastSyncedAt: row.last_synced_at ? String(row.last_synced_at) : null,
    lastError: row.last_error ? String(row.last_error) : null,
    createdAt: row.created_at ? String(row.created_at) : "",
    updatedAt: row.updated_at ? String(row.updated_at) : "",
  };
}

export function mapWorkflow(row: SupabaseRow): Workflow {
  return {
    id: String(row.id),
    projectId: row.project_id ? String(row.project_id) : null,
    name: String(row.name),
    description: String(row.description),
    isTemplate: Boolean(row.is_template),
    providerOverride: row.provider_override ? String(row.provider_override) : null,
    modelOverride: row.model_override
      ? normalizeStepModel(String(row.model_override)) ?? String(row.model_override)
      : null,
    reasoningEffortOverride: row.reasoning_effort_override ? String(row.reasoning_effort_override) : null,
    yoloMode: Boolean(row.yolo_mode),
    createdBy: String(row.created_by),
    createdAt: String(row.created_at),
    updatedAt: String(row.updated_at),
  };
}

export function mapWorkflowStep(row: SupabaseRow): WorkflowStep {
  return {
    id: String(row.id),
    workflowId: String(row.workflow_id),
    stepType: row.step_type as StepType,
    orderIndex: Number(row.order_index),
    isEnabled: Boolean(row.is_enabled),
    providerOverride: row.provider_override ? String(row.provider_override) : null,
    modelOverride: row.model_override
      ? normalizeStepModel(String(row.model_override)) ?? String(row.model_override)
      : null,
    reasoningEffortOverride: row.reasoning_effort_override ? String(row.reasoning_effort_override) : null,
    requiresApproval: Boolean(row.requires_approval),
    createdAt: String(row.created_at),
    updatedAt: String(row.updated_at),
  };
}

export function mapWorkflowRun(row: SupabaseRow): WorkflowRun {
  return {
    id: String(row.id),
    workflowId: String(row.workflow_id),
    projectId: String(row.project_id),
    status: row.status as WorkflowRunStatus,
    provider: row.provider ? String(row.provider) : null,
    model: row.model ? normalizeStepModel(String(row.model)) ?? String(row.model) : null,
    reasoningEffort: row.reasoning_effort ? (String(row.reasoning_effort) as ReasoningEffort) : null,
    yoloMode: Boolean(row.yolo_mode),
    startedBy: String(row.started_by),
    startedAt: String(row.started_at),
    finishedAt: row.finished_at ? String(row.finished_at) : null,
    errorMessage: row.error_message ? String(row.error_message) : null,
  };
}

export function mapWorkflowRunStep(row: SupabaseRow): WorkflowRunStep {
  return {
    id: String(row.id),
    workflowRunId: String(row.workflow_run_id),
    workflowStepId: row.workflow_step_id ? String(row.workflow_step_id) : null,
    executionOrderIndex: Number(row.execution_order_index ?? 0),
    stepType: row.step_type as StepType,
    status: row.status as WorkflowStepStatus,
    artifactId: row.artifact_id ? String(row.artifact_id) : null,
    artifactRunId: row.artifact_run_id ? String(row.artifact_run_id) : null,
    promptCacheId: row.prompt_cache_id ? String(row.prompt_cache_id) : null,
    rejectionNote: row.rejection_note ? String(row.rejection_note) : null,
    retryCount: Number(row.retry_count || 0),
    startedAt: row.started_at ? String(row.started_at) : null,
    finishedAt: row.finished_at ? String(row.finished_at) : null,
    errorMessage: row.error_message ? String(row.error_message) : null,
  };
}

export function mapWorkflowRunLog(row: SupabaseRow): WorkflowRunLog {
  return {
    id: String(row.id),
    workflowRunStepId: String(row.workflow_run_step_id),
    logLevel: row.log_level as "info" | "warn" | "error" | "debug",
    message: String(row.message),
    createdAt: String(row.created_at),
  };
}

export function mapWorkflowRunSession(row: SupabaseRow): WorkflowRunSession {
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

export function mapSupportedModel(row: SupabaseRow): SupportedModel {
  return {
    id: String(row.id),
    providerKey: row.provider_key as "codex" | "claude" | "gemini",
    modelId: String(row.model_id),
    displayName: String(row.display_name),
    isEnabled: Boolean(row.is_enabled),
    sortOrder: Number(row.sort_order),
    source: String(row.source),
    detectionMethod: row.detection_method ? String(row.detection_method) : null,
    detectedCliVersion: row.detected_cli_version ? String(row.detected_cli_version) : null,
    lastDetectedAt: row.last_detected_at ? String(row.last_detected_at) : null,
    createdAt: row.created_at ? String(row.created_at) : "",
    updatedAt: row.updated_at ? String(row.updated_at) : "",
  };
}
