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
}

interface WorkflowStepDefinitionRow {
  id: string;
  is_enabled: boolean;
  requires_approval: boolean;
}

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
  if (!email) {
    throw new Error("Authenticated user must have an email address.");
  }

  const { data, error } = await adminClient
    .from("project_teams")
    .select("project_id, team_members!inner(email)")
    .eq("project_id", projectId)
    .ilike("team_members.email", email)
    .limit(1);

  if (error) {
    throw new Error(`Unable to verify project access: ${error.message}`);
  }

  if (!data || data.length === 0) {
    throw new Error("Forbidden");
  }
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
): Promise<{ id: string; provider_override: string | null; model_override: string | null }> {
  const { data, error } = await adminClient
    .from("workflows")
    .select("id, provider_override, model_override")
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
  }>
> {
  const { data, error } = await adminClient
    .from("workflow_steps")
    .select("id, step_type, order_index, is_enabled, requires_approval")
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
  }>;
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
      .select("id, is_enabled, requires_approval")
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
      requiresApproval: definition?.requires_approval ?? true,
      startedAt: step.started_at,
      retryCount: step.retry_count,
      rejectionNote: step.rejection_note,
    };
  });
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
