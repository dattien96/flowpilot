import { NextResponse } from "next/server";

import { assertAdminApiSession } from "@/data/auth/session";
import { getLocalRunnerBaseUrl } from "@/lib/env/app-env";

export async function POST(request: Request) {
  try {
    const auth = await assertAdminApiSession();
    if (!auth.ok) return auth.response;

    const payload = (await request.json()) as { projectId?: string };
    const projectId = payload.projectId?.trim() ?? "";
    if (!projectId) {
      return NextResponse.json({ error: "projectId is required." }, { status: 400 });
    }

    const response = await fetch(
      new URL("/artifact-storage/google-drive/connect-sessions", getLocalRunnerBaseUrl()),
      {
        method: "POST",
        cache: "no-store",
        headers: {
          "content-type": "application/json",
        },
        body: JSON.stringify({
          projectId,
          baseUrl: getLocalRunnerBaseUrl(),
        }),
      },
    );

    const body = (await response.json()) as Record<string, unknown>;
    if (!response.ok) {
      return NextResponse.json(
        { error: String(body.error ?? "Unable to create Google Drive connect session.") },
        { status: response.status },
      );
    }

    return NextResponse.json(body);
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Unable to create Google Drive connect session.",
      },
      { status: 400 },
    );
  }
}
