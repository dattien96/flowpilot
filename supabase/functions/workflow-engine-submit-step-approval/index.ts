import {
  assertStepAccess,
  createWorkflowClients,
  getWorkflowRunStep,
  progressWorkflowRun,
  requireAuthenticatedUser,
  resetStepForRetry,
} from "../_shared/workflow-engine-runtime.ts";
import {
  workflowCorsPreflightResponse,
  workflowJsonResponse,
  workflowTextResponse,
} from "../_shared/cors.ts";

const APPROVAL_DECISION_LOG_PREFIX = "approval_decision:";

Deno.serve(async (req) => {
  if (req.method === "OPTIONS") return workflowCorsPreflightResponse();
  if (req.method !== "POST") return workflowTextResponse("Method not allowed", { status: 405 });

  const authHeader = req.headers.get("Authorization");
  if (!authHeader) return workflowTextResponse("Unauthorized", { status: 401 });

  try {
    const { authClient, adminClient } = createWorkflowClients(authHeader);
    await requireAuthenticatedUser(authClient);

    const body = await req.json();
    const stepId = String(body?.stepId ?? "");
    const approve = Boolean(body?.approve);
    const comment = body?.comment ? String(body.comment) : "Rejected by reviewer.";
    if (!stepId) return workflowTextResponse("Missing stepId", { status: 400 });

    await assertStepAccess(authClient, stepId);

    const currentStep = await getWorkflowRunStep(adminClient, stepId);
    if (currentStep.status !== "WAITING_USER_APPROVAL") {
      if (approve) {
        return workflowTextResponse("Step is not waiting for approval.", { status: 400 });
      }
      if (currentStep.status !== "DONE") {
        return workflowTextResponse("Can only request changes on waiting or completed steps.", { status: 400 });
      }
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

      if (error) return workflowTextResponse(error.message, { status: 400 });

      const { error: logError } = await adminClient.from("workflow_run_logs").insert({
        workflow_run_step_id: stepId,
        log_level: "info",
        message: `${APPROVAL_DECISION_LOG_PREFIX}${JSON.stringify({
          id: crypto.randomUUID(),
          approvalId: `approval_${stepId}`,
          workflowRunId: currentStep.workflow_run_id,
          workflowStepId: stepId,
          aiOutputId: null,
          decision: "approved",
          reviewerId: null,
          comment: body?.comment ? String(body.comment) : null,
          createdAt: now,
        })}`,
        created_at: now,
      });
      if (logError) return workflowTextResponse(logError.message, { status: 400 });

      await progressWorkflowRun(adminClient, currentStep.workflow_run_id);
    } else {
      const now = new Date().toISOString();
      const { error: logError } = await adminClient.from("workflow_run_logs").insert({
        workflow_run_step_id: stepId,
        log_level: "info",
        message: `${APPROVAL_DECISION_LOG_PREFIX}${JSON.stringify({
          id: crypto.randomUUID(),
          approvalId: `approval_${stepId}`,
          workflowRunId: currentStep.workflow_run_id,
          workflowStepId: stepId,
          aiOutputId: null,
          decision: "changes_requested",
          reviewerId: null,
          comment,
          createdAt: now,
        })}`,
        created_at: now,
      });
      if (logError) return workflowTextResponse(logError.message, { status: 400 });

      const runId = await resetStepForRetry(adminClient, stepId, comment);
      await progressWorkflowRun(adminClient, runId);
    }

    const updatedStep = await getWorkflowRunStep(adminClient, stepId);
    return workflowJsonResponse(updatedStep);
  } catch (error) {
    const message = error instanceof Error ? error.message : "Unknown error";
    const status = message === "Unauthorized" ? 401 : message === "Forbidden" ? 403 : 400;
    return workflowTextResponse(message, { status });
  }
});
