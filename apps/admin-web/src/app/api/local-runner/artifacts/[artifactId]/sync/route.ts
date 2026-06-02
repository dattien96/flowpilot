import { NextResponse } from "next/server";

import { assertAdminApiSession } from "@/data/auth/session";
import { createGatewayBundle } from "@/data/repository/factory";
import { SupabaseArtifactStorageConnectionGateway } from "@/data/repository/supabase/supabase-artifact-storage-connection-gateway";
import { createSupabaseServerClient } from "@/data/datasource/supabase/client";
import { getArtifactStorageConnection } from "@/features/artifacts/artifact-storage-connection";
import { getLocalRunnerBaseUrl } from "@/lib/env/app-env";

type RouteContext = {
  params: Promise<{
    artifactId: string;
  }>;
};

async function updateSharedArtifactMetadata(
  artifactId: string,
  artifact: {
    storageProvider?: string;
    remotePath?: string;
    remoteObjectId?: string;
    syncStatus?: string;
  },
) {
  try {
    const supabase = createSupabaseServerClient();
    const gateway = new SupabaseArtifactStorageConnectionGateway(supabase);
    await gateway.updateArtifactRunStorageMetadata({
      artifactRunId: artifactId,
      storageProvider:
        artifact.storageProvider === "google_drive" ? "google_drive" : "supabase",
      remotePath: artifact.remotePath ?? "",
      remoteObjectId: artifact.remoteObjectId ?? null,
      syncStatus:
        artifact.syncStatus === "synced" || artifact.syncStatus === "failed"
          ? artifact.syncStatus
          : undefined,
    });
  } catch {
    // Best-effort only.
  }
}

export async function POST(_: Request, context: RouteContext) {
  try {
    const auth = await assertAdminApiSession();
    if (!auth.ok) return auth.response;

    const { artifactId } = await context.params;
    const gateways = await createGatewayBundle();
    const artifact = await gateways.localRunnerGateway.getArtifactById(artifactId);
    if (!artifact) {
      return NextResponse.json({ error: "Artifact not found." }, { status: 404 });
    }

    const project = await gateways.projectGateway.getProjectById(artifact.projectId);
    const storageProvider = project?.artifactStoragePreference ?? "supabase";
    let googleDrivePayload:
      | {
          googleDriveProjectId: string;
          googleDriveFolderId: string;
        }
      | undefined;

    if (storageProvider === "google_drive") {
      const supabase = createSupabaseServerClient();
      const connection = await getArtifactStorageConnection(
        supabase,
        artifact.projectId,
        "google_drive",
      );
      if (!connection || connection.status !== "connected" || !connection.folderId.trim()) {
        return NextResponse.json(
          {
            error:
              "Google Drive artifact storage is not connected on this runner for the selected project.",
          },
          { status: 400 },
        );
      }
      googleDrivePayload = {
        googleDriveProjectId: artifact.projectId,
        googleDriveFolderId: connection.folderId,
      };
    }

    const response = await fetch(new URL(`/artifacts/${artifactId}/sync`, getLocalRunnerBaseUrl()), {
      method: "POST",
      cache: "no-store",
      headers: {
        "content-type": "application/json",
      },
      body: JSON.stringify({
        storageProvider,
        ...googleDrivePayload,
      }),
    });

    const payload = (await response.json()) as
      | {
          error?: string;
        }
      | {
          storageProvider?: string;
          remotePath?: string;
          remoteObjectId?: string;
          syncStatus?: string;
        };

    if (!response.ok) {
      return NextResponse.json(
        {
          error:
            "error" in payload && payload.error ? payload.error : "Unable to sync artifact.",
        },
        { status: response.status },
      );
    }

    await updateSharedArtifactMetadata(artifactId, payload);
    return NextResponse.json(payload);
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Unable to sync artifact.",
      },
      { status: 400 },
    );
  }
}
