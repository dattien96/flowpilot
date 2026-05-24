import {
  assertRunAccess,
  createWorkflowClients,
  getWorkflowRun,
  progressWorkflowRun,
  requireAuthenticatedUser,
} from "../_shared/workflow-engine-runtime.ts";
import {
  workflowCorsPreflightResponse,
  workflowJsonResponse,
  workflowTextResponse,
} from "../_shared/cors.ts";

Deno.serve(async (req) => {
  if (req.method === "OPTIONS") return workflowCorsPreflightResponse();
  if (req.method !== "POST") return workflowTextResponse("Method not allowed", { status: 405 });

  const authHeader = req.headers.get("Authorization");
  if (!authHeader) return workflowTextResponse("Unauthorized", { status: 401 });

  try {
    const { authClient, adminClient } = createWorkflowClients(authHeader);
    await requireAuthenticatedUser(authClient);

    const body = await req.json();
    const runId = String(body?.runId ?? "");
    const yoloMode = Boolean(body?.yoloMode);
    if (!runId) return workflowTextResponse("Missing runId", { status: 400 });

    await assertRunAccess(authClient, runId);

    const { error } = await adminClient
      .from("workflow_runs")
      .update({ yolo_mode: yoloMode })
      .eq("id", runId);

    if (error) return workflowTextResponse(error.message, { status: 400 });

    const run = yoloMode
      ? await progressWorkflowRun(adminClient, runId)
      : await getWorkflowRun(adminClient, runId);

    return workflowJsonResponse(run);
  } catch (error) {
    const message = error instanceof Error ? error.message : "Unknown error";
    const status = message === "Unauthorized" ? 401 : message === "Forbidden" ? 403 : 400;
    return workflowTextResponse(message, { status });
  }
});
