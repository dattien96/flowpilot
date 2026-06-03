import { NextResponse } from "next/server";
import { createClient } from "@supabase/supabase-js";

import { createGatewayBundle } from "@/data/repository/factory";
import { SupabaseArtifactStorageConnectionGateway } from "@/data/repository/supabase/supabase-artifact-storage-connection-gateway";
import { syncArtifactThroughRunner } from "@/features/artifacts/artifact-sync-service";
import { getRequiredEnv, getSupabaseUrl } from "@/lib/env/app-env";

type RouteContext = {
  params: Promise<{
    artifactId: string;
  }>;
};

function createSupabaseAdminClient() {
  return createClient(
    getSupabaseUrl(),
    process.env.SUPABASE_SERVICE_ROLE_KEY ??
      process.env.SUPABASE_API_SERVICE_ROLE_KEY ??
      getRequiredEnv("SUPABASE_SERVICE_ROLE_KEY"),
  );
}

export async function POST(_: Request, context: RouteContext) {
  try {
    const { artifactId } = await context.params;
    const gateways = await createGatewayBundle();
    const artifact = await gateways.localRunnerGateway.getArtifactById(artifactId);

    if (!artifact) {
      return NextResponse.json({ error: "Artifact not found." }, { status: 404 });
    }

    const supabase = createSupabaseAdminClient();
    const storageGateway = new SupabaseArtifactStorageConnectionGateway(supabase);
    const payload = await syncArtifactThroughRunner(artifactId, {
      supabase,
      localRunnerGateway: {
        getArtifactById: gateways.localRunnerGateway.getArtifactById.bind(
          gateways.localRunnerGateway,
        ),
        listArtifacts: gateways.localRunnerGateway.listArtifacts.bind(
          gateways.localRunnerGateway,
        ),
        exportArtifactSyncBundle: gateways.localRunnerGateway.exportArtifactSyncBundle.bind(
          gateways.localRunnerGateway,
        ),
        saveArtifactCloudSyncResult: gateways.localRunnerGateway.saveArtifactCloudSyncResult.bind(
          gateways.localRunnerGateway,
        ),
      },
      artifactStorageConnectionGateway: storageGateway,
    });

    if (payload?.syncStatus === "failed") {
      await storageGateway.updateArtifactRunStorageMetadata({
        artifactRunId: artifactId,
        storageProvider:
          payload.storageProvider === "google_drive" ? "google_drive" : "supabase",
        remotePath: payload.remotePath ?? undefined,
        remoteObjectId: payload.remoteObjectId ?? undefined,
        syncStatus: "failed",
      });
    }

    const updatedArtifact = await gateways.localRunnerGateway.getArtifactById(artifactId);
    return NextResponse.json(updatedArtifact ?? artifact);
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Unable to sync artifact.",
      },
      { status: 400 },
    );
  }
}
