import { redirect } from "next/navigation";
import { NextResponse } from "next/server";
import { z } from "zod";

import { createGatewayBundle } from "@/data/repository/factory";
import { StartWorkflowRunUseCase } from "@/domain/usecase/workflow-runs/start-workflow-run-usecase";

const startWorkflowSchema = z.object({
  featureId: z.string().min(1),
  workflowDefinitionId: z.string().min(1),
  contextSourceIds: z.string().optional(),
});

export async function POST(request: Request) {
  const contentType = request.headers.get("content-type") ?? "";
  let parsedInput: z.infer<typeof startWorkflowSchema>;

  if (contentType.includes("application/json")) {
    parsedInput = startWorkflowSchema.parse(await request.json());
  } else {
    const formData = await request.formData();
    parsedInput = startWorkflowSchema.parse({
      featureId: formData.get("featureId"),
      workflowDefinitionId: formData.get("workflowDefinitionId"),
      contextSourceIds: formData.get("contextSourceIds"),
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
    contextSourceIds: parsedInput.contextSourceIds
      ? parsedInput.contextSourceIds.split(",").filter(Boolean)
      : [],
  });

  if (!result) {
    return NextResponse.json({ error: "Unable to create workflow run." }, { status: 500 });
  }

  if (contentType.includes("application/json")) {
    return NextResponse.json(result, { status: 201 });
  }

  redirect(`/workflow-runs/${result.run.id}`);
}
