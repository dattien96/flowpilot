import { NextResponse } from "next/server";

import { createGatewayBundle } from "@/data/repository/factory";
import { SupabaseArtifactStorageConnectionGateway } from "@/data/repository/supabase/supabase-artifact-storage-connection-gateway";
import { switchProjectArtifactStorageProvider } from "@/features/artifacts/artifact-provider-switch-service";
import { createRuntimeSupabaseAdminClient } from "@/lib/supabase/runtime-config.server";

export async function POST(request: Request) {
  try {
    const payload = (await request.json()) as {
      projectId?: string;
      targetProvider?: string;
    };

    const projectId = payload.projectId?.trim() ?? "";
    const targetProvider = payload.targetProvider?.trim() ?? "";
    if (!projectId) {
      return NextResponse.json({ error: "projectId is required." }, { status: 400 });
    }
    if (!targetProvider) {
      return NextResponse.json({ error: "targetProvider is required." }, { status: 400 });
    }

    const supabase = await createRuntimeSupabaseAdminClient();
    const gateways = await createGatewayBundle();
    const storageGateway = new SupabaseArtifactStorageConnectionGateway(supabase);
    const result = await switchProjectArtifactStorageProvider(projectId, targetProvider, {
      supabase,
      localRunnerGateway: {
        getArtifactById: gateways.localRunnerGateway.getArtifactById.bind(gateways.localRunnerGateway),
        listArtifacts: gateways.localRunnerGateway.listArtifacts.bind(gateways.localRunnerGateway),
        exportArtifactSyncBundle: gateways.localRunnerGateway.exportArtifactSyncBundle.bind(gateways.localRunnerGateway),
        saveArtifactCloudSyncResult: gateways.localRunnerGateway.saveArtifactCloudSyncResult.bind(gateways.localRunnerGateway),
        hydrateArtifactFromRemote: gateways.localRunnerGateway.hydrateArtifactFromRemote.bind(gateways.localRunnerGateway),
      },
      artifactStorageConnectionGateway: storageGateway,
    });

    return NextResponse.json(result);
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Unable to switch artifact storage provider.",
      },
      { status: 400 },
    );
  }
}
