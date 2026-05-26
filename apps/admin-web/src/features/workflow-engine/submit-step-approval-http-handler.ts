import { createClient } from "@supabase/supabase-js";
import { z } from "zod";

import { createGatewayBundle } from "@/data/repository/factory";
import { submitWorkflowStepFollowUpRuntime } from "@/features/workflow-engine/workflow-start-runtime";
import { getRequiredEnv, getSupabaseUrl, hasSupabaseEnv } from "@/lib/env/app-env";

const submitDecisionSchema = z.object({
  stepId: z.string().min(1),
  approve: z.boolean(),
  comment: z.string().optional(),
});

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

export async function handleWorkflowSubmitStepApproval(request: Request) {
  try {
    await requireApiUser(request);
    const payload = submitDecisionSchema.parse(await request.json());
    if (payload.approve) {
      throw new Error("Approve flow must use the workflow edge function.");
    }

    const gateways = await createGatewayBundle();
    const step = await submitWorkflowStepFollowUpRuntime({
      adminClient: createAdminClient(),
      localRunnerGateway: gateways.localRunnerGateway,
      stepId: payload.stepId,
      comment: payload.comment ?? "",
    });

    return Response.json(step);
  } catch (error) {
    const message = error instanceof Error ? error.message : "Unable to submit follow-up prompt.";
    const status = message === "Unauthorized" ? 401 : message === "Forbidden" ? 403 : 400;
    return Response.json({ error: message }, { status });
  }
}
