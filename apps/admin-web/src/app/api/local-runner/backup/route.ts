import { NextResponse } from "next/server";
import { z } from "zod";

import { assertAdminApiSession } from "@/data/auth/session";
import { createGatewayBundle } from "@/data/repository/factory";
import { CreateArtifactBackupUseCase } from "@/domain/usecase/artifacts/create-artifact-backup-usecase";

const backupSchema = z.object({
  scope: z.string().default("all"),
  runId: z.string().nullable().optional(),
});

export async function POST(request: Request) {
  try {
    const auth = await assertAdminApiSession();
    if (!auth.ok) return auth.response;

    const payload = backupSchema.parse(await request.json());
    const gateways = await createGatewayBundle();
    const result = await new CreateArtifactBackupUseCase(gateways.localRunnerGateway).execute(
      payload.scope,
      payload.runId ?? null,
    );
    return NextResponse.json(result);
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Unable to create backup.",
      },
      { status: 400 },
    );
  }
}
