import { createClient } from "@supabase/supabase-js";
import { z } from "zod";

import { createGatewayBundle } from "@/data/repository/factory";
import { STEP_MODEL_OPTIONS } from "@/domain/model/entity/workflow-engine";
import { runWorkflowStartRuntime } from "@/features/workflow-engine/workflow-start-runtime";
import { getRequiredEnv, getSupabaseUrl, hasSupabaseEnv } from "@/lib/env/app-env";

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
  if (!hasSupabaseEnv()) {
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

  const authClient = createClient(getSupabaseUrl(), getRequiredEnv("SUPABASE_API_KEY"));
  const { data, error } = await authClient.auth.getUser(token);
  if (error || !data.user) {
    throw new Error("Unauthorized");
  }

  return {
    id: data.user.id,
    email: data.user.email ?? null,
  };
}

function createAdminClient() {
  return createClient(
    getSupabaseUrl(),
    process.env.SUPABASE_SERVICE_ROLE_KEY ??
      process.env.SUPABASE_API_SERVICE_ROLE_KEY ??
      getRequiredEnv("SUPABASE_SERVICE_ROLE_KEY"),
  );
}

export async function handleWorkflowStartRun(request: Request) {
  try {
    const user = await requireApiUser(request);
    const payload = startRunSchema.parse(await request.json());
    const supportedModels = STEP_MODEL_OPTIONS.map((option) => option.value).join(", ");
    const gateways = await createGatewayBundle();
    const run = await runWorkflowStartRuntime({
      adminClient: createAdminClient(),
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
