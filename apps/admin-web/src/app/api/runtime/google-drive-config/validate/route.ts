import { proxyRunnerJSON } from "../_shared";

export async function POST(request: Request) {
  try {
    const payload = await request.json();
    return proxyRunnerJSON("/google-drive-config/validate", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(payload),
    });
  } catch (error) {
    return Response.json(
      {
        error: error instanceof Error ? error.message : "Unable to validate Google Drive config.",
      },
      { status: 400 },
    );
  }
}
