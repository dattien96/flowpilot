import { NextResponse } from "next/server";

import { assertAdminApiSession } from "@/data/auth/session";
import { createSupabaseServerClient } from "@/data/datasource/supabase/client";
import {
  upsertArtifactStorageConnection,
} from "@/features/artifacts/artifact-storage-connection";
import { getLocalRunnerBaseUrl } from "@/lib/env/app-env";

export async function GET(request: Request) {
  try {
    const auth = await assertAdminApiSession();
    if (!auth.ok) return auth.response;

    const { searchParams } = new URL(request.url);
    const projectId = searchParams.get("projectId")?.trim() ?? "";
    const sessionId = searchParams.get("sessionId")?.trim() ?? "";
    if (!projectId) {
      return NextResponse.json({ error: "projectId is required." }, { status: 400 });
    }

    const runnerURL = new URL("/artifact-storage/google-drive/connection", getLocalRunnerBaseUrl());
    runnerURL.searchParams.set("projectId", projectId);
    if (sessionId) {
      runnerURL.searchParams.set("sessionId", sessionId);
    }

    const response = await fetch(runnerURL, {
      method: "GET",
      cache: "no-store",
    });
    const body = (await response.json()) as Record<string, any>;
    if (!response.ok) {
      return NextResponse.json(
        { error: String(body.error ?? "Unable to read Google Drive status.") },
        { status: response.status },
      );
    }

    const connection = body.connection as Record<string, unknown> | undefined;
    if (connection && typeof connection.status === "string") {
      try {
        await upsertArtifactStorageConnection(createSupabaseServerClient(), {
          projectId,
          provider: "google_drive",
          status:
            connection.status === "connected"
              ? "connected"
              : connection.status === "failed"
                ? "failed"
                : "pending",
          folderId: typeof connection.folderId === "string" ? connection.folderId : null,
          folderName: typeof connection.folderName === "string" ? connection.folderName : null,
          oauthAccountEmail:
            typeof connection.accountEmail === "string" ? connection.accountEmail : null,
          lastValidatedAt:
            typeof connection.lastValidatedAt === "string" ? connection.lastValidatedAt : null,
          lastError: typeof connection.lastError === "string" ? connection.lastError : null,
          connectedAt: typeof connection.connectedAt === "string" ? connection.connectedAt : null,
        });
      } catch {
        // Best effort only.
      }
    }

    return NextResponse.json(body);
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Unable to read Google Drive status.",
      },
      { status: 400 },
    );
  }
}
