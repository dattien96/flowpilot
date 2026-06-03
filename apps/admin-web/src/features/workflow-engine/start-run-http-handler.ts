import { z } from "zod";

import { createGatewayBundle } from "@/data/repository/factory";
import { STEP_MODEL_OPTIONS } from "@/domain/model/entity/workflow-engine";
import { runWorkflowStartRuntime } from "@/features/workflow-engine/workflow-start-runtime";
import {
  createRuntimeSupabaseAdminClient,
  createRuntimeSupabaseAnonClient,
  hasSupabaseRuntimeConfigOrEnvFallback,
} from "@/lib/supabase/runtime-config.server";

const startRunSchema = z.discriminatedUnion("startMode", [
  z.object({
    workflowId: z.string().min(1),
    projectId: z.string().min(1),
    startMode: z.literal("workflow-definition"),
    beginPrompt: z.string().min(1),
    stepType: z.null().optional(),
  }),
  z.object({
    workflowId: z.string().min(1).optional().nullable(),
    projectId: z.string().min(1),
    startMode: z.literal("single-step"),
    beginPrompt: z.string().min(1),
    stepType: z.string().min(1),
  }),
]);

async function requireApiUser(request: Request) {
  if (!(await hasSupabaseRuntimeConfigOrEnvFallback())) {
    return {
      id: "demo-user",
      email: "demo@flowpilot.local",
    };
  }

  const authHeader = request.headers.get("authorization");
  const token = authHeader?.replace(/^Bearer\s+/i, "").trim();
  if (!token) {
    throw new Error("Unauthorized");
  }

  const authClient = await createRuntimeSupabaseAnonClient();
  const { data, error } = await authClient.auth.getUser(token);
  if (error || !data.user) {
    throw new Error("Unauthorized");
  }

  return {
    id: data.user.id,
    email: data.user.email ?? null,
  };
}

export async function handleWorkflowStartRun(request: Request) {
  try {
    const user = await requireApiUser(request);
    const payload = startRunSchema.parse(await request.json());
    const gateways = await createGatewayBundle();
    const dbSupportedModels = await gateways.workflowEngineGateway.listSupportedModels();
    const supportedModels = dbSupportedModels.length > 0
      ? dbSupportedModels.map((m) => m.modelId).join(", ")
      : STEP_MODEL_OPTIONS.map((option) => option.value).join(", ");
    
    const run = await runWorkflowStartRuntime({
      adminClient: await createRuntimeSupabaseAdminClient(),
      localRunnerGateway: gateways.localRunnerGateway,
      request: payload,
      user,
    });

    return Response.json(run, {
      headers: {
        "x-workflow-step-models": supportedModels,
      },
    });
  } catch (error) {
    const message = error instanceof Error ? error.message : "Unable to start workflow run.";
    const status = message === "Unauthorized" ? 401 : message === "Forbidden" ? 403 : 400;
    return Response.json({ error: message }, { status });
  }
}
