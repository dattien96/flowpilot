import { NextResponse } from "next/server";

import { localRunnerRequest, readRunnerError } from "../../_shared";

export async function POST(request: Request) {
  try {
    const payload = (await request.json().catch(() => ({}))) as { accountId?: string };
    const response = await localRunnerRequest("/google-drive-config/accounts/connect-sessions", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(payload ?? {}),
    });
    if (!response.ok) {
      return NextResponse.json(
        { error: await readRunnerError(response) },
        { status: response.status },
      );
    }
    return NextResponse.json(await response.json());
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Unable to start Google Drive account connection.",
      },
      { status: 400 },
    );
  }
}
