import type {
  ArtifactDefinition,
  ArtifactRun,
  StepDefinition,
  Workflow,
  WorkflowStep,
  WorkflowRun,
  WorkflowRunStep,
  WorkflowRunLog,
  StepType,
  WorkflowRunStatus,
  WorkflowStepStatus,
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
    requiredMcps: mcps,
    requiredSkills: skills,
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
  return {
    id: String(row.id),
    artifactDefinitionKey: String(row.artifact_definition_key ?? row.artifact_key),
    workflowId: String(row.workflow_id),
    workflowRunId: String(row.workflow_run_id),
    workflowRunStepId: row.workflow_run_step_id ? String(row.workflow_run_step_id) : null,
    projectId: row.project_id ? String(row.project_id) : null,
    title: String(row.title),
    localPath: String(row.local_path),
    remotePath: row.remote_path ? String(row.remote_path) : "",
    remoteUrl: row.remote_url ? String(row.remote_url) : "",
    syncStatus: row.sync_status as ArtifactRun["syncStatus"],
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
    modelOverride: row.model_override ? String(row.model_override) : null,
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
    modelOverride: row.model_override ? String(row.model_override) : null,
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
    model: row.model ? String(row.model) : null,
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
