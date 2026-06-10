import { NextResponse } from "next/server";

import { assertAdminApiSessionForRequest } from "@/data/auth/session";
import { getLocalRunnerBaseUrl } from "@/lib/env/app-env";

async function readRunnerResponse(response: Response) {
  const raw = await response.text();
  if (!raw.trim()) {
    return { raw, body: {} as Record<string, unknown> };
  }

  try {
    return {
      raw,
      body: JSON.parse(raw) as Record<string, unknown>,
    };
  } catch {
    return {
      raw,
      body: { error: raw } satisfies Record<string, unknown>,
    };
  }
}

export async function POST(request: Request) {
  try {
    const auth = await assertAdminApiSessionForRequest(request);
    if (!auth.ok) return auth.response;

    const payload = (await request.json()) as { projectId?: string; accountId?: string };
    const projectId = payload.projectId?.trim() ?? "";
    const accountId = payload.accountId?.trim() ?? "";
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
          accountId: accountId || undefined,
        }),
      },
    );

    const { body } = await readRunnerResponse(response);
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
