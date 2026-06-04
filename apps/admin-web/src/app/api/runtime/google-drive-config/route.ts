import { NextResponse } from "next/server";

import {
  browserSafeStatus,
  localRunnerRequest,
  readRunnerError,
  resolveGoogleDriveRuntimeStatus,
  proxyRunnerJSON,
} from "./_shared";

export async function GET() {
  const status = await resolveGoogleDriveRuntimeStatus();
  return NextResponse.json(browserSafeStatus(status), {
    headers: { "cache-control": "no-store" },
  });
}

export async function PUT(request: Request) {
  try {
    const payload = await request.json();
    const response = await localRunnerRequest("/google-drive-config", {
      method: "PUT",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(payload),
    });
    if (!response.ok) {
      return NextResponse.json(
        { error: await readRunnerError(response) },
        { status: response.status },
      );
    }
    const status = await resolveGoogleDriveRuntimeStatus();
    return NextResponse.json(browserSafeStatus(status));
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Unable to save Google Drive config.",
      },
      { status: 400 },
    );
  }
}

export async function DELETE() {
  return proxyRunnerJSON("/google-drive-config", { method: "DELETE" });
}
