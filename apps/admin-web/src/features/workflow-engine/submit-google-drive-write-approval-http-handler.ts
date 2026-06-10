import { z } from "zod";

import { assertAdminApiSessionForRequest } from "@/data/auth/session";
import { createGatewayBundle } from "@/data/repository/factory";
import { submitGoogleDriveWriteApprovalRuntime } from "@/features/workflow-engine/workflow-start-runtime";
import {
  createRuntimeSupabaseAdminClient,
} from "@/lib/supabase/runtime-config.server";

const submitGoogleDriveWriteApprovalSchema = z.object({
  stepId: z.string().min(1),
  decision: z.enum(["approved", "rejected"]),
  comment: z.string().optional(),
});

async function parseApprovalPayload(request: Request) {
  const contentType = request.headers.get("content-type") ?? "";
  if (contentType.includes("application/json")) {
    return {
      wantsJson: true,
      payload: submitGoogleDriveWriteApprovalSchema.parse(await request.json()),
    };
  }

  const formData = await request.formData();
  return {
    wantsJson: false,
    payload: submitGoogleDriveWriteApprovalSchema.parse({
      stepId: formData.get("stepId"),
      decision: formData.get("decision"),
      comment: formData.get("comment"),
    }),
  };
}

export async function handleWorkflowSubmitGoogleDriveWriteApproval(
  request: Request,
) {
  try {
    const auth = await assertAdminApiSessionForRequest(request);
    if (!auth.ok) {
      return auth.response;
    }

    const { wantsJson, payload } = await parseApprovalPayload(request);
    const gateways = await createGatewayBundle();
    const step = await submitGoogleDriveWriteApprovalRuntime({
      adminClient: await createRuntimeSupabaseAdminClient(),
      localRunnerGateway: gateways.localRunnerGateway,
      stepId: payload.stepId,
      decision: payload.decision,
      comment: payload.comment,
    });

    if (wantsJson) {
      return Response.json(step);
    }

    const referer = request.headers.get("referer")?.trim();
    return Response.redirect(
      referer ? new URL(referer) : new URL("/workflow-runs", request.url),
      303,
    );
  } catch (error) {
    const message = error instanceof Error ? error.message : "Unable to submit Google Drive MCP approval.";
    const status = message === "Unauthorized" ? 401 : message === "Forbidden" ? 403 : 400;
    return Response.json({ error: message }, { status });
  }
}
