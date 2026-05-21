import type {
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

export function mapStepDefinition(row: SupabaseRow): StepDefinition {
  const mcps = Array.isArray(row.required_mcps)
    ? row.required_mcps.map(String)
    : [];
  const skills = Array.isArray(row.required_skills)
    ? row.required_skills.map(String)
    : [];

  return {
    stepType: row.step_type as StepType,
    name: String(row.name),
    description: String(row.description),
    requiredMcps: mcps,
    requiredSkills: skills,
    agentType: row.agent_type as "standard" | "autonomous",
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
