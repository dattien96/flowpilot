import { createClient, type SupabaseClient } from "https://esm.sh/@supabase/supabase-js@2";

import {
  planRejectedStepRetry,
  planWorkflowProgress,
  type RuntimeWorkflowStep,
  type WorkflowStepPatch,
} from "./workflow-engine-state-machine.ts";

interface WorkflowRunRow {
  id: string;
  workflow_id: string;
  project_id: string;
  status: "PENDING" | "RUNNING" | "DONE" | "FAILED" | "CANCELED";
  provider: string | null;
  model: string | null;
  yolo_mode: boolean;
  started_by: string;
  started_at: string;
  finished_at: string | null;
  error_message: string | null;
}

interface ProjectSettingsRow {
  default_provider: string | null;
  default_model: string | null;
  default_reasoning_effort: string | null;
}

interface WorkflowRunStepRow {
  id: string;
  workflow_run_id: string;
  workflow_step_id: string | null;
  execution_order_index: number;
  step_type: string;
  status: RuntimeWorkflowStep["status"];
  rejection_note: string | null;
  retry_count: number;
  started_at: string | null;
  finished_at: string | null;
  artifact_run_id: string | null;
}

interface WorkflowStepDefinitionRow {
  id?: string;
  step_type: string;
  name: string;
  model: string | null;
  reasoning_effort: string | null;
  requires_approval?: boolean;
  yolo_mode?: boolean | null;
}

interface StepArtifactBindingRow {
  artifact_definition_key: string;
  order_index: number;
}

interface ArtifactDefinitionRow {
  key: string;
  name: string;
  description: string;
  local_path_template: string;
  remote_path_template: string;
  default_file_name: string;
}

const SINGLE_STEP_RUNTIME_CREATED_BY = "flowpilot-runtime";
const DEFAULT_MODEL = "gpt-5.4";
const DEFAULT_REASONING_EFFORT = "medium";

export function createWorkflowClients(authHeader: string) {
  const supabaseUrl = Deno.env.get("SUPABASE_URL") ?? "";
  const anonKey = Deno.env.get("SUPABASE_ANON_KEY") ?? "";
  const serviceRoleKey = Deno.env.get("SUPABASE_SERVICE_ROLE_KEY") ?? "";

  const authClient = createClient(supabaseUrl, anonKey, {
    global: { headers: { Authorization: authHeader } },
  });
  const adminClient = createClient(supabaseUrl, serviceRoleKey);

  return { authClient, adminClient };
}

export async function requireAuthenticatedUser(authClient: SupabaseClient) {
  const {
    data: { user },
    error,
  } = await authClient.auth.getUser();

  if (error || !user) {
    throw new Error("Unauthorized");
  }

  return user;
}

export async function assertProjectMembership(
  adminClient: SupabaseClient,
  projectId: string,
  email: string | null | undefined
) {
  // Since this is the admin app, authenticated users have access to all projects.
  // We just verify that the project actually exists.
  const { data, error } = await adminClient
    .from("projects")
    .select("id")
    .eq("id", projectId)
    .maybeSingle();

  if (error) {
    throw new Error(`Unable to verify project: ${error.message}`);
  }

  if (!data) {
    throw new Error("Project not found");
  }
}

export async function loadProjectDefaults(
  adminClient: SupabaseClient,
  projectId: string
): Promise<ProjectSettingsRow> {
  const { data, error } = await adminClient
    .from("projects")
    .select("default_provider, default_model, default_reasoning_effort")
    .eq("id", projectId)
    .maybeSingle();

  if (error) {
    throw new Error(`Unable to load project defaults: ${error.message}`);
  }

  return (data ?? { default_provider: null, default_model: null, default_reasoning_effort: null }) as ProjectSettingsRow;
}

export function resolveProviderKeyFromModel(model: string) {
  if (model.startsWith("gpt-")) {
    return "codex";
  }
  if (
    model.startsWith("gemini-") ||
    model.startsWith("auto-gemini-") ||
    model === "gemini-pro" ||
    model === "gemini-flash"
  ) {
    return "gemini";
  }
  if (model.startsWith("claude-")) {
    return "claude";
  }

  throw new Error(`Model "${model}" is not supported by the workflow runner.`);
}

export async function assertRunAccess(authClient: SupabaseClient, runId: string) {
  const { data, error } = await authClient
    .from("workflow_runs")
    .select("id")
    .eq("id", runId)
    .maybeSingle();

  if (error) {
    throw new Error(`Unable to verify run access: ${error.message}`);
  }

  if (!data) {
    throw new Error("Forbidden");
  }
}

export async function assertStepAccess(authClient: SupabaseClient, stepId: string) {
  const { data, error } = await authClient
    .from("workflow_run_steps")
    .select("id")
    .eq("id", stepId)
    .maybeSingle();

  if (error) {
    throw new Error(`Unable to verify step access: ${error.message}`);
  }

  if (!data) {
    throw new Error("Forbidden");
  }
}

export async function getWorkflowRun(
  adminClient: SupabaseClient,
  runId: string
): Promise<WorkflowRunRow> {
  const { data, error } = await adminClient
    .from("workflow_runs")
    .select("*")
    .eq("id", runId)
    .single();

  if (error) {
    throw new Error(`Unable to load workflow run: ${error.message}`);
  }

  return data as WorkflowRunRow;
}

export async function getWorkflowRunStep(
  adminClient: SupabaseClient,
  stepId: string
): Promise<WorkflowRunStepRow> {
  const { data, error } = await adminClient
    .from("workflow_run_steps")
    .select("*")
    .eq("id", stepId)
    .single();

  if (error) {
    throw new Error(`Unable to load workflow run step: ${error.message}`);
  }

  return data as WorkflowRunStepRow;
}

export async function getWorkflowDefinition(
  adminClient: SupabaseClient,
  workflowId: string,
  projectId: string
): Promise<{
  id: string;
  provider_override: string | null;
  model_override: string | null;
  reasoning_effort_override: string | null;
}> {
  const { data, error } = await adminClient
    .from("workflows")
    .select("id, provider_override, model_override, reasoning_effort_override")
    .eq("id", workflowId)
    .or(`project_id.is.null,project_id.eq.${projectId}`)
    .single();

  if (error) {
    throw new Error(`Unable to load workflow definition: ${error.message}`);
  }

  return data;
}

export async function getWorkflowDefinitionSteps(
  adminClient: SupabaseClient,
  workflowId: string
): Promise<
  Array<{
    id: string;
    step_type: string;
    order_index: number;
    is_enabled: boolean;
    requires_approval: boolean;
    model_override: string | null;
    reasoning_effort_override: string | null;
  }>
> {
  const { data, error } = await adminClient
    .from("workflow_steps")
    .select("id, step_type, order_index, is_enabled, requires_approval, model_override, reasoning_effort_override")
    .eq("workflow_id", workflowId)
    .order("order_index", { ascending: true });

  if (error) {
    throw new Error(`Unable to load workflow steps: ${error.message}`);
  }

  return (data ?? []) as Array<{
    id: string;
    step_type: string;
    order_index: number;
    is_enabled: boolean;
    requires_approval: boolean;
    model_override: string | null;
    reasoning_effort_override: string | null;
  }>;
}

export async function createSingleStepWorkflow(
  adminClient: SupabaseClient,
  projectId: string,
  stepType: string
): Promise<{
  workflow: {
    id: string;
    provider_override: string | null;
    model_override: string | null;
    reasoning_effort_override: string | null;
    yolo_mode?: boolean | null;
  };
  workflowSteps: Array<{
    id: string;
    step_type: string;
    order_index: number;
    is_enabled: boolean;
    yolo_mode?: boolean | null;
    requires_approval: boolean;
  }>;
}> {
  const { data: definitionData, error: definitionError } = await adminClient
    .from("step_definitions")
    .select("step_type, name, model, reasoning_effort, yolo_mode")
    .eq("step_type", stepType)
    .maybeSingle();

  if (definitionError) {
    throw new Error(`Unable to load step definition: ${definitionError.message}`);
  }
  if (!definitionData) {
    throw new Error(`Step definition "${stepType}" was not found.`);
  }

  const definition = definitionData as WorkflowStepDefinitionRow;
  const resolvedModel = definition.model ?? DEFAULT_MODEL;
  const resolvedReasoningEffort = definition.reasoning_effort ?? DEFAULT_REASONING_EFFORT;
  const singleStepYoloMode =
    typeof definition.yolo_mode === "boolean" ? definition.yolo_mode : false;
  const { data: workflowData, error: workflowError } = await adminClient
    .from("workflows")
    .insert({
      project_id: projectId,
      name: `Single Step: ${definition.name}`,
      description: `Runtime-generated single-step workflow for ${definition.step_type}.`,
      is_template: false,
      provider_override: resolveProviderKeyFromModel(resolvedModel),
      model_override: resolvedModel,
      reasoning_effort_override: resolvedReasoningEffort,
      yolo_mode: singleStepYoloMode,
      created_by: SINGLE_STEP_RUNTIME_CREATED_BY,
    })
    .select("id, provider_override, model_override, reasoning_effort_override, yolo_mode")
    .single();

  if (workflowError) {
    throw new Error(`Unable to create single-step workflow: ${workflowError.message}`);
  }

  const { data: workflowStepData, error: workflowStepError } = await adminClient
    .from("workflow_steps")
    .insert({
      workflow_id: workflowData.id,
      step_type: definition.step_type,
      order_index: 0,
      is_enabled: true,
      provider_override: resolveProviderKeyFromModel(resolvedModel),
      model_override: resolvedModel,
      reasoning_effort_override: resolvedReasoningEffort,
      yolo_mode: singleStepYoloMode,
      requires_approval: false,
    })
    .select("id, step_type, order_index, is_enabled, yolo_mode, requires_approval")
    .single();

  if (workflowStepError) {
    throw new Error(`Unable to create single-step workflow step: ${workflowStepError.message}`);
  }

  return {
    workflow: workflowData as {
      id: string;
      provider_override: string | null;
      model_override: string | null;
      reasoning_effort_override: string | null;
      yolo_mode?: boolean | null;
    },
    workflowSteps: [
      workflowStepData as {
        id: string;
        step_type: string;
        order_index: number;
        is_enabled: boolean;
        yolo_mode?: boolean | null;
        requires_approval: boolean;
      },
    ],
  };
}

async function listRuntimeSteps(
  adminClient: SupabaseClient,
  runId: string
): Promise<RuntimeWorkflowStep[]> {
  const { data: runStepsData, error: runStepsError } = await adminClient
    .from("workflow_run_steps")
    .select("*")
    .eq("workflow_run_id", runId)
    .order("execution_order_index", { ascending: true });

  if (runStepsError) {
    throw new Error(`Unable to load workflow run steps: ${runStepsError.message}`);
  }

  const runSteps = (runStepsData ?? []) as WorkflowRunStepRow[];
  const workflowStepIds = Array.from(
    new Set(runSteps.map((step) => step.workflow_step_id).filter(Boolean))
  ) as string[];

  let definitionsById = new Map<string, WorkflowStepDefinitionRow>();
  if (workflowStepIds.length > 0) {
    const { data: definitionData, error: definitionError } = await adminClient
      .from("workflow_steps")
      .select("id, is_enabled, requires_approval, yolo_mode")
      .in("id", workflowStepIds);

    if (definitionError) {
      throw new Error(`Unable to load workflow step definitions: ${definitionError.message}`);
    }

    definitionsById = new Map(
      ((definitionData ?? []) as WorkflowStepDefinitionRow[]).map((row) => [row.id, row])
    );
  }

  return runSteps.map((step) => {
    const definition = step.workflow_step_id
      ? definitionsById.get(step.workflow_step_id) ?? null
      : null;

    return {
      id: step.id,
      stepType: step.step_type,
      status: step.status,
      yoloMode:
        typeof definition?.yolo_mode === "boolean" ? definition.yolo_mode : null,
      requiresApproval: definition?.requires_approval ?? true,
      startedAt: step.started_at,
      retryCount: step.retry_count,
      rejectionNote: step.rejection_note,
    };
  });
}

function resolveArtifactPath(
  template: string,
  context: {
    projectId: string;
    workflowId: string;
    workflowRunId: string;
    workflowRunStepId: string;
    stepType: string;
    artifactKey: string;
    defaultFileName: string;
  }
) {
  return template
    .replaceAll("{projectId}", context.projectId)
    .replaceAll("{workflowId}", context.workflowId)
    .replaceAll("{workflowRunId}", context.workflowRunId)
    .replaceAll("{workflowRunStepId}", context.workflowRunStepId)
    .replaceAll("{stepType}", context.stepType)
    .replaceAll("{artifactKey}", context.artifactKey)
    .replaceAll("{defaultFileName}", context.defaultFileName);
}

async function createArtifactRunsForStep(
  adminClient: SupabaseClient,
  run: WorkflowRunRow,
  step: RuntimeWorkflowStep
) {
  const { data: bindingData, error: bindingError } = await adminClient
    .from("step_output_artifact_definitions")
    .select("artifact_definition_key, order_index")
    .eq("step_type", step.stepType)
    .order("order_index", { ascending: true });

  if (bindingError) {
    throw new Error(`Unable to load step artifact outputs: ${bindingError.message}`);
  }

  const bindings = (bindingData ?? []) as StepArtifactBindingRow[];
  if (bindings.length === 0) {
    return;
  }

  const definitionKeys = bindings.map((binding) => binding.artifact_definition_key);
  const { data: artifactDefinitions, error: definitionsError } = await adminClient
    .from("artifact_definitions")
    .select("key, name, description, local_path_template, remote_path_template, default_file_name")
    .in("key", definitionKeys);

  if (definitionsError) {
    throw new Error(`Unable to load artifact definitions: ${definitionsError.message}`);
  }

  const definitionsByKey = new Map(
    ((artifactDefinitions ?? []) as ArtifactDefinitionRow[]).map((definition) => [
      definition.key,
      definition,
    ])
  );

  const now = new Date().toISOString();
  const artifactRows = bindings.map((binding) => {
    const definition = definitionsByKey.get(binding.artifact_definition_key);
    if (!definition) {
      throw new Error(
        `Missing artifact definition for key "${binding.artifact_definition_key}".`
      );
    }

    const context = {
      projectId: run.project_id,
      workflowId: run.workflow_id,
      workflowRunId: run.id,
      workflowRunStepId: step.id,
      stepType: step.stepType,
      artifactKey: definition.key,
      defaultFileName: definition.default_file_name || definition.name,
    };

    return {
      artifact_definition_key: definition.key,
      project_id: run.project_id,
      workflow_id: run.workflow_id,
      workflow_run_id: run.id,
      workflow_run_step_id: step.id,
      title: definition.default_file_name || definition.name,
      local_path: resolveArtifactPath(definition.local_path_template, context),
      remote_path: resolveArtifactPath(definition.remote_path_template, context),
      remote_url: "",
      sync_status: "local_only",
      created_at: now,
      updated_at: now,
    };
  });

  const { data: createdArtifacts, error: insertError } = await adminClient
    .from("artifact_runs")
    .insert(artifactRows)
    .select("*");

  if (insertError) {
    throw new Error(`Unable to create artifact runs: ${insertError.message}`);
  }

  const firstCreatedArtifact = createdArtifacts?.[0];
  if (firstCreatedArtifact) {
    const { error: stepUpdateError } = await adminClient
      .from("workflow_run_steps")
      .update({ artifact_run_id: firstCreatedArtifact.id })
      .eq("id", step.id);

    if (stepUpdateError) {
      throw new Error(`Unable to attach artifact run to workflow step: ${stepUpdateError.message}`);
    }
  }
}

function toStepDbPatch(patch: WorkflowStepPatch) {
  return {
    status: patch.status,
    started_at: patch.startedAt,
    finished_at: patch.finishedAt,
    rejection_note: patch.rejectionNote,
    retry_count: patch.retryCount,
  };
}

export async function insertWorkflowRunLog(
  adminClient: SupabaseClient,
  workflowRunStepId: string,
  logLevel: "info" | "warn" | "error" | "debug",
  message: string
) {
  const { error } = await adminClient.from("workflow_run_logs").insert({
    workflow_run_step_id: workflowRunStepId,
    log_level: logLevel,
    message,
  });

  if (error) {
    throw new Error(`Unable to write workflow run log: ${error.message}`);
  }
}

async function applyStepPatch(
  adminClient: SupabaseClient,
  stepId: string,
  patch: WorkflowStepPatch
) {
  const { error } = await adminClient
    .from("workflow_run_steps")
    .update(toStepDbPatch(patch))
    .eq("id", stepId);

  if (error) {
    throw new Error(`Unable to update workflow run step: ${error.message}`);
  }
}

export async function progressWorkflowRun(adminClient: SupabaseClient, runId: string) {
  const run = await getWorkflowRun(adminClient, runId);
  const runtimeSteps = await listRuntimeSteps(adminClient, runId);
  const now = new Date().toISOString();
  const plan = planWorkflowProgress({
    steps: runtimeSteps,
    yoloMode: run.yolo_mode,
    now,
  });

  for (const transition of plan.stepTransitions) {
    await applyStepPatch(adminClient, transition.stepId, transition.patch);
    for (const log of transition.logs) {
      await insertWorkflowRunLog(adminClient, transition.stepId, log.logLevel, log.message);
    }
    if (transition.patch.status === "DONE") {
      const completedStep = runtimeSteps.find((step) => step.id === transition.stepId);
      if (completedStep) {
        await createArtifactRunsForStep(adminClient, run, completedStep);
      }
    }
  }

  const { error: runUpdateError } = await adminClient
    .from("workflow_runs")
    .update({
      status: plan.runStatus,
      finished_at: plan.runFinishedAt,
      error_message: null,
    })
    .eq("id", runId);

  if (runUpdateError) {
    throw new Error(`Unable to update workflow run: ${runUpdateError.message}`);
  }

  return getWorkflowRun(adminClient, runId);
}

export async function resetStepForRetry(
  adminClient: SupabaseClient,
  stepId: string,
  comment: string
) {
  const currentStep = await getWorkflowRunStep(adminClient, stepId);
  const transition = planRejectedStepRetry({
    stepId,
    step: {
      status: currentStep.status,
      retryCount: currentStep.retry_count,
    },
    now: new Date().toISOString(),
    comment,
  });

  await applyStepPatch(adminClient, stepId, transition.patch);
  for (const log of transition.logs) {
    await insertWorkflowRunLog(adminClient, stepId, log.logLevel, log.message);
  }

  return currentStep.workflow_run_id;
}
