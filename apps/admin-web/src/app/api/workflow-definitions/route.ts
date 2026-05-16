import { NextResponse } from "next/server";

import { assertAdminApiSession } from "@/data/auth/session";
import { createGatewayBundle } from "@/data/repository/factory";
import { ListWorkflowDefinitionsUseCase } from "@/domain/usecase/workflow-definitions/list-workflow-definitions-usecase";

export async function GET() {
  const auth = await assertAdminApiSession();
  if (!auth.ok) return auth.response;

  const gateways = await createGatewayBundle();
  const definitions = await new ListWorkflowDefinitionsUseCase(
    gateways.workflowGateway,
  ).execute();

  return NextResponse.json({ definitions });
}
