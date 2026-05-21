import {
  assertRunAccess,
  createWorkflowClients,
  getWorkflowRun,
  progressWorkflowRun,
  requireAuthenticatedUser,
} from "../_shared/workflow-engine-runtime.ts";

Deno.serve(async (req) => {
  if (req.method !== "POST") return new Response("Method not allowed", { status: 405 });

  const authHeader = req.headers.get("Authorization");
  if (!authHeader) return new Response("Unauthorized", { status: 401 });

  try {
    const { authClient, adminClient } = createWorkflowClients(authHeader);
    await requireAuthenticatedUser(authClient);

    const body = await req.json();
    const runId = String(body?.runId ?? "");
    const yoloMode = Boolean(body?.yoloMode);
    if (!runId) return new Response("Missing runId", { status: 400 });

    await assertRunAccess(authClient, runId);

    const { error } = await adminClient
      .from("workflow_runs")
      .update({ yolo_mode: yoloMode })
      .eq("id", runId);

    if (error) return new Response(error.message, { status: 400 });

    const run = yoloMode
      ? await progressWorkflowRun(adminClient, runId)
      : await getWorkflowRun(adminClient, runId);

    return Response.json(run);
  } catch (error) {
    const message = error instanceof Error ? error.message : "Unknown error";
    const status = message === "Unauthorized" ? 401 : message === "Forbidden" ? 403 : 400;
    return new Response(message, { status });
  }
});
