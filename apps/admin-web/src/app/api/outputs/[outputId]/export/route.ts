import { NextResponse } from "next/server";

import { assertAdminApiSession } from "@/data/auth/session";
import { createGatewayBundle } from "@/data/repository/factory";
import { ExportOutputUseCase } from "@/domain/usecase/outputs/export-output-usecase";

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
  const exported = await new ExportOutputUseCase(gateways.workflowGateway).execute(
    outputId,
  );

  return new NextResponse(exported.contentMarkdown, {
    headers: {
      "content-disposition": `attachment; filename="${exported.filename}"`,
      "content-type": "text/markdown; charset=utf-8",
    },
  });
}
