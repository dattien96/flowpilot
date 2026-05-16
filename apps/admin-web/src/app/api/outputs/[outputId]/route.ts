import { NextResponse } from "next/server";

import { assertAdminApiSession } from "@/data/auth/session";
import { createGatewayBundle } from "@/data/repository/factory";
import { GetOutputDetailUseCase } from "@/domain/usecase/outputs/get-output-detail-usecase";

type RouteContext = {
  params: Promise<{
    outputId: string;
  }>;
};

export async function GET(_: Request, context: RouteContext) {
  const auth = await assertAdminApiSession();
  if (!auth.ok) return auth.response;

  const { outputId } = await context.params;
  const gateways = await createGatewayBundle();
  const output = await new GetOutputDetailUseCase(
    gateways.workflowGateway,
  ).execute(outputId);

  if (!output) {
    return NextResponse.json({ error: "Output not found." }, { status: 404 });
  }

  return NextResponse.json(output);
}
