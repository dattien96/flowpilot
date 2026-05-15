import { NextResponse } from "next/server";

import { createGatewayBundle } from "@/data/repository/factory";
import { SyncArtifactUseCase } from "@/domain/usecase/artifacts/sync-artifact-usecase";

type RouteContext = {
  params: Promise<{
    artifactId: string;
  }>;
};

export async function POST(_: Request, context: RouteContext) {
  try {
    const { artifactId } = await context.params;
    const gateways = await createGatewayBundle();
    const result = await new SyncArtifactUseCase(gateways.localRunnerGateway).execute(artifactId);
    return NextResponse.json(result);
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Unable to sync artifact.",
      },
      { status: 400 },
    );
  }
}
