import { NextResponse } from "next/server";

import { SupabaseArtifactStorageConnectionGateway } from "@/data/repository/supabase/supabase-artifact-storage-connection-gateway";
import { triggerArtifactSyncBootstrap } from "@/features/artifacts/artifact-auto-sync";
import { createSupabaseAdminClient } from "@/lib/supabase/admin";

export async function POST() {
  try {
    console.info("[artifact-sync] bootstrap requested");
    const supabase = await createSupabaseAdminClient();
    triggerArtifactSyncBootstrap({
      supabase,
      artifactStorageConnectionGateway: new SupabaseArtifactStorageConnectionGateway(
        supabase,
      ),
    });
    console.info("[artifact-sync] bootstrap scheduled");
    return NextResponse.json({ started: true });
  } catch (error) {
    console.warn("[artifact-sync] bootstrap failed", error);
    return NextResponse.json(
      {
        error:
          error instanceof Error
            ? error.message
            : "Unable to start artifact sync bootstrap.",
      },
      { status: 400 },
    );
  }
}
