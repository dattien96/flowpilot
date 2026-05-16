import { redirect } from "next/navigation";
import { NextResponse } from "next/server";
import { z } from "zod";

import { assertAdminApiSession } from "@/data/auth/session";
import { createGatewayBundle } from "@/data/repository/factory";
import { StartWorkflowRunUseCase } from "@/domain/usecase/workflow-runs/start-workflow-run-usecase";

const startWorkflowSchema = z.object({
  featureId: z.string().min(1),
  workflowDefinitionId: z.string().min(1),
  contextSourceIds: z.union([z.string(), z.array(z.string())]).optional(),
});

function normalizeContextSourceIds(input: string | string[] | undefined) {
  if (!input) {
    return [];
  }

  const values = Array.isArray(input) ? input : input.split(",");
  return values.map((value) => value.trim()).filter(Boolean);
}

export async function POST(request: Request) {
  const auth = await assertAdminApiSession();
  if (!auth.ok) return auth.response;

  const contentType = request.headers.get("content-type") ?? "";
  let parsedInput: z.infer<typeof startWorkflowSchema>;

  if (contentType.includes("application/json")) {
    parsedInput = startWorkflowSchema.parse(await request.json());
  } else {
    const formData = await request.formData();
    parsedInput = startWorkflowSchema.parse({
      featureId: formData.get("featureId"),
      workflowDefinitionId: formData.get("workflowDefinitionId"),
      contextSourceIds: formData.getAll("contextSourceIds").map(String),
    });
  }

  const gateways = await createGatewayBundle();
  const result = await new StartWorkflowRunUseCase(
    gateways.featureGateway,
    gateways.workflowGateway,
    gateways.workflowExecutor,
  ).execute({
    featureId: parsedInput.featureId,
    workflowDefinitionId: parsedInput.workflowDefinitionId,
    contextSourceIds: normalizeContextSourceIds(parsedInput.contextSourceIds),
  });

  if (!result) {
    return NextResponse.json({ error: "Unable to create workflow run." }, { status: 500 });
  }

  if (contentType.includes("application/json")) {
    return NextResponse.json(result, { status: 201 });
  }

  redirect(`/workflow-runs/${result.run.id}`);
}
