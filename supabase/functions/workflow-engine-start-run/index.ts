import {
  assertProjectMembership,
  createWorkflowClients,
  getWorkflowDefinition,
  getWorkflowDefinitionSteps,
  progressWorkflowRun,
  requireAuthenticatedUser,
} from "../_shared/workflow-engine-runtime.ts";

Deno.serve(async (req) => {
  if (req.method !== "POST") {
    return new Response("Method not allowed", { status: 405 });
  }

  const authHeader = req.headers.get("Authorization");
  if (!authHeader) {
    return new Response("Unauthorized", { status: 401 });
  }

  try {
    const { authClient, adminClient } = createWorkflowClients(authHeader);
    const user = await requireAuthenticatedUser(authClient);
    const body = await req.json();
    const workflowId = String(body?.workflowId ?? "");
    const projectId = String(body?.projectId ?? "");

    if (!workflowId || !projectId) {
      return new Response("Missing workflowId or projectId", { status: 400 });
    }

    await assertProjectMembership(adminClient, projectId, user.email);
    const workflow = await getWorkflowDefinition(adminClient, workflowId, projectId);
    const workflowSteps = await getWorkflowDefinitionSteps(adminClient, workflowId);

    const { data: runData, error: runError } = await adminClient
      .from("workflow_runs")
      .insert({
        workflow_id: workflowId,
        project_id: projectId,
        status: "RUNNING",
        provider: workflow.provider_override,
        model: workflow.model_override,
        yolo_mode: false,
        started_by: user.email ?? user.id,
      })
      .select("*")
      .single();

    if (runError) {
      return new Response(runError.message, { status: 400 });
    }

    if (workflowSteps.length > 0) {
      const { error: stepsError } = await adminClient.from("workflow_run_steps").insert(
        workflowSteps.map((step) => ({
          workflow_run_id: runData.id,
          workflow_step_id: step.id,
          execution_order_index: step.order_index,
          step_type: step.step_type,
          status: step.is_enabled ? "PENDING" : "SKIPPED",
          retry_count: 0,
        }))
      );

      if (stepsError) {
        return new Response(stepsError.message, { status: 400 });
      }
    }

    const run = await progressWorkflowRun(adminClient, runData.id);
    return Response.json(run);
  } catch (error) {
    const message = error instanceof Error ? error.message : "Unknown error";
    const status = message === "Unauthorized" ? 401 : message === "Forbidden" ? 403 : 400;
    return new Response(message, { status });
  }
});
