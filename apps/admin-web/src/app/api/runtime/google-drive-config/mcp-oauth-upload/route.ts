import { NextResponse } from "next/server";

import { localRunnerRequest, readGoogleDriveUploadForm, readRunnerError, resolveGoogleDriveRuntimeStatus } from "../_shared";

export async function POST(request: Request) {
  try {
    const payload = await readGoogleDriveUploadForm(request);
    const response = await localRunnerRequest("/google-drive-config/mcp-oauth-upload", {
      method: "POST",
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
    return NextResponse.json(status);
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Unable to upload Google Drive MCP OAuth JSON.",
      },
      { status: 400 },
    );
  }
}
