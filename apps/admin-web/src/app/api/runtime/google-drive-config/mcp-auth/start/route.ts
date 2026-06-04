import { NextResponse } from "next/server";

import { localRunnerRequest, readRunnerError } from "../../_shared";

export async function POST() {
  const response = await localRunnerRequest("/google-drive-config/mcp-auth/start", {
    method: "POST",
  });

  if (!response.ok) {
    return NextResponse.json(
      { error: await readRunnerError(response) },
      { status: response.status },
    );
  }

  const payload = await response.json().catch(() => ({}));
  return NextResponse.json(payload);
}
