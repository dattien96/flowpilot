import { z } from "zod";

import { createGatewayBundle } from "@/data/repository/factory";
import { submitGoogleDriveWriteApprovalRuntime } from "@/features/workflow-engine/workflow-start-runtime";
import {
  createRuntimeSupabaseAdminClient,
  createRuntimeSupabaseAnonClient,
  hasSupabaseRuntimeConfigOrEnvFallback,
} from "@/lib/supabase/runtime-config.server";

const submitGoogleDriveWriteApprovalSchema = z.object({
  stepId: z.string().min(1),
  decision: z.enum(["approved", "rejected"]),
  comment: z.string().optional(),
});

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

export async function handleWorkflowSubmitGoogleDriveWriteApproval(
  request: Request,
) {
  try {
    await requireApiUser(request);
    const payload = submitGoogleDriveWriteApprovalSchema.parse(await request.json());
    const gateways = await createGatewayBundle();
    const step = await submitGoogleDriveWriteApprovalRuntime({
      adminClient: await createRuntimeSupabaseAdminClient(),
      localRunnerGateway: gateways.localRunnerGateway,
      stepId: payload.stepId,
      decision: payload.decision,
      comment: payload.comment,
    });

    return Response.json(step);
  } catch (error) {
    const message = error instanceof Error ? error.message : "Unable to submit Google Drive write approval.";
    const status = message === "Unauthorized" ? 401 : message === "Forbidden" ? 403 : 400;
    return Response.json({ error: message }, { status });
  }
}
