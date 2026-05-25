import { NextResponse } from "next/server";
import { z } from "zod";

import { assertAdminApiSession } from "@/data/auth/session";
import { createGatewayBundle } from "@/data/repository/factory";
import { ListOutputsUseCase } from "@/domain/usecase/outputs/list-outputs-usecase";

const outputFiltersSchema = z.object({
  projectId: z.string().optional(),
  workflowRunId: z.string().optional(),
  outputType: z
    .enum([
      "business_summary",
      "product_spec",
      "android_tech_spec",
      "task_breakdown",
      "test_plan",
      "risk_report",
    ])
    .optional(),
  approvalState: z.enum(["approved", "pending"]).optional(),
});

function readFilters(request: Request) {
  const url = new URL(request.url);
  return outputFiltersSchema.parse({
    projectId: url.searchParams.get("projectId") || undefined,
    workflowRunId: url.searchParams.get("workflowRunId") || undefined,
    outputType: url.searchParams.get("outputType") || undefined,
    approvalState: url.searchParams.get("approvalState") || undefined,
  });
}

export async function GET(request: Request) {
  const auth = await assertAdminApiSession();
  if (!auth.ok) return auth.response;

  const filters = readFilters(request);
  const gateways = await createGatewayBundle();
  const outputs = await new ListOutputsUseCase(gateways.workflowGateway).execute(filters);

  return NextResponse.json({ outputs });
}
