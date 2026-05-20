import {
  assertStepAccess,
  createWorkflowClients,
  getWorkflowRunStep,
  progressWorkflowRun,
  requireAuthenticatedUser,
  resetStepForRetry,
} from "../_shared/workflow-engine-runtime.ts";

Deno.serve(async (req) => {
  if (req.method !== "POST") return new Response("Method not allowed", { status: 405 });

  const authHeader = req.headers.get("Authorization");
  if (!authHeader) return new Response("Unauthorized", { status: 401 });

  try {
    const { authClient, adminClient } = createWorkflowClients(authHeader);
    await requireAuthenticatedUser(authClient);

    const body = await req.json();
    const stepId = String(body?.stepId ?? "");
    const approve = Boolean(body?.approve);
    const comment = body?.comment ? String(body.comment) : "Rejected by reviewer.";
    if (!stepId) return new Response("Missing stepId", { status: 400 });

    await assertStepAccess(authClient, stepId);

    const currentStep = await getWorkflowRunStep(adminClient, stepId);
    if (currentStep.status !== "WAITING_USER_APPROVAL") {
      return new Response("Step is not waiting for approval.", { status: 400 });
    }

    if (approve) {
      const now = new Date().toISOString();
      const { error } = await adminClient
        .from("workflow_run_steps")
        .update({
          status: "DONE",
          started_at: currentStep.started_at ?? now,
          finished_at: now,
          rejection_note: null,
        })
        .eq("id", stepId);

      if (error) return new Response(error.message, { status: 400 });

      await progressWorkflowRun(adminClient, currentStep.workflow_run_id);
    } else {
      const runId = await resetStepForRetry(adminClient, stepId, comment);
      await progressWorkflowRun(adminClient, runId);
    }

    const updatedStep = await getWorkflowRunStep(adminClient, stepId);
    return Response.json(updatedStep);
  } catch (error) {
    const message = error instanceof Error ? error.message : "Unknown error";
    const status = message === "Unauthorized" ? 401 : message === "Forbidden" ? 403 : 400;
    return new Response(message, { status });
  }
});
