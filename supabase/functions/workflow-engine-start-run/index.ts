import {
  assertProjectMembership,
  createSingleStepWorkflow,
  createWorkflowClients,
  getWorkflowDefinition,
  getWorkflowDefinitionSteps,
  loadProjectDefaults,
  progressWorkflowRun,
  resolveProviderKeyFromModel,
  requireAuthenticatedUser,
} from "../_shared/workflow-engine-runtime.ts";
import {
  workflowCorsPreflightResponse,
  workflowJsonResponse,
  workflowTextResponse,
} from "../_shared/cors.ts";

Deno.serve(async (req) => {
  if (req.method === "OPTIONS") {
    return workflowCorsPreflightResponse();
  }

  if (req.method !== "POST") {
    return workflowTextResponse("Method not allowed", { status: 405 });
  }

  const authHeader = req.headers.get("Authorization");
  if (!authHeader) {
    return workflowTextResponse("Unauthorized", { status: 401 });
  }

  try {
    const { authClient, adminClient } = createWorkflowClients(authHeader);
    const user = await requireAuthenticatedUser(authClient);
    const body = await req.json();
    const projectId = String(body?.projectId ?? "");
    const startMode = String(body?.startMode ?? "workflow-definition");
    const beginPrompt = String(body?.beginPrompt ?? "").trim();
    const workflowId = body?.workflowId ? String(body.workflowId) : "";
    const stepType = body?.stepType ? String(body.stepType) : "";

    if (!projectId) {
      return workflowTextResponse("Missing projectId", { status: 400 });
    }
    if (startMode !== "workflow-definition" && startMode !== "single-step") {
      return workflowTextResponse("Invalid startMode", { status: 400 });
    }
    if (startMode === "workflow-definition" && !workflowId) {
      return workflowTextResponse("Missing workflowId", { status: 400 });
    }
    if (startMode === "single-step" && !stepType) {
      return workflowTextResponse("Missing stepType", { status: 400 });
    }

    await assertProjectMembership(adminClient, projectId, user.email);
    const projectDefaults = await loadProjectDefaults(adminClient, projectId);
    const executionSource =
      startMode === "single-step"
        ? await createSingleStepWorkflow(adminClient, projectId, stepType)
        : {
            workflow: await getWorkflowDefinition(adminClient, workflowId, projectId),
            workflowSteps: await getWorkflowDefinitionSteps(adminClient, workflowId),
          };
    const { workflow, workflowSteps } = executionSource;

    const stepTypes = Array.from(
      new Set(workflowSteps.map((step) => step.step_type)),
    );
    const { data: stepDefinitionRows, error: stepDefinitionError } = stepTypes.length > 0
      ? await adminClient
          .from("step_definitions")
          .select("step_type, model, reasoning_effort")
          .in("step_type", stepTypes)
      : { data: [], error: null };

    if (stepDefinitionError) {
      return workflowTextResponse(stepDefinitionError.message, { status: 400 });
    }

    const stepDefinitionsByType = new Map(
      (stepDefinitionRows ?? []).map((row) => [
        String(row.step_type),
        {
          model: String(row.model ?? ""),
          reasoningEffort: String(row.reasoning_effort ?? ""),
        },
      ]),
    );
    const firstEnabledStep = workflowSteps.find((step) => step.is_enabled) ?? workflowSteps[0] ?? null;
    const firstStepModel = firstEnabledStep
      ? firstEnabledStep.model_override ||
        workflow.model_override ||
        projectDefaults.default_model ||
        stepDefinitionsByType.get(firstEnabledStep.step_type)?.model ||
        "gpt-5.4"
      : workflow.model_override || projectDefaults.default_model || "gpt-5.4";
    const resolvedProvider = resolveProviderKeyFromModel(firstStepModel);
    const resolvedReasoningEffort =
      (firstEnabledStep
        ? firstEnabledStep.reasoning_effort_override ||
          workflow.reasoning_effort_override ||
          projectDefaults.default_reasoning_effort ||
          stepDefinitionsByType.get(firstEnabledStep.step_type)?.reasoningEffort ||
          "medium"
        : workflow.reasoning_effort_override ||
          projectDefaults.default_reasoning_effort ||
          "medium") || "medium";

    const { data: runData, error: runError } = await adminClient
      .from("workflow_runs")
      .insert({
        workflow_id: workflow.id,
        project_id: projectId,
        status: "RUNNING",
        provider: resolvedProvider,
        model: firstStepModel,
        reasoning_effort: resolvedReasoningEffort,
        yolo_mode: Boolean(workflow.yolo_mode),
        started_by: user.email ?? user.id,
      })
      .select("*")
      .single();

    if (runError) {
      return workflowTextResponse(runError.message, { status: 400 });
    }

    const { data: insertedSteps, error: stepsError } = workflowSteps.length > 0
      ? await adminClient
          .from("workflow_run_steps")
          .insert(
            workflowSteps.map((step) => ({
              workflow_run_id: runData.id,
              workflow_step_id: step.id,
              execution_order_index: step.order_index,
              step_type: step.step_type,
              status: step.is_enabled ? "PENDING" : "SKIPPED",
              retry_count: 0,
            }))
          )
          .select("*")
      : { data: [], error: null };

    if (stepsError) {
      return workflowTextResponse(stepsError.message, { status: 400 });
    }

    if (beginPrompt && insertedSteps && insertedSteps.length > 0) {
      const firstExecutableStep =
        insertedSteps.find((step) => step.status !== "SKIPPED") ?? insertedSteps[0];

      if (firstExecutableStep) {
        const { error: promptLogError } = await adminClient.from("workflow_run_logs").insert({
          workflow_run_step_id: firstExecutableStep.id,
          log_level: "info",
          message: `Begin prompt: ${beginPrompt}`,
        });

        if (promptLogError) {
          return workflowTextResponse(promptLogError.message, { status: 400 });
        }
      }
    }

    const run = await progressWorkflowRun(adminClient, runData.id);
    return workflowJsonResponse(run);
  } catch (error) {
    const message = error instanceof Error ? error.message : "Unknown error";
    const status = message === "Unauthorized" ? 401 : message === "Forbidden" ? 403 : 400;
    return workflowTextResponse(message, { status });
  }
});
