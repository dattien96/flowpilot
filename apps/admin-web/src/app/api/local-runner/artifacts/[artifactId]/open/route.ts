import { NextResponse } from "next/server";

import { assertAdminApiSession } from "@/data/auth/session";
import { createGatewayBundle } from "@/data/repository/factory";
import { getLocalRunnerBaseUrl } from "@/lib/env/app-env";

type RouteContext = {
  params: Promise<{
    artifactId: string;
  }>;
};

export async function GET(_: Request, context: RouteContext) {
  const auth = await assertAdminApiSession();
  if (!auth.ok) return auth.response;

  const { artifactId } = await context.params;
  const gateways = await createGatewayBundle();
  const artifact = await gateways.localRunnerGateway.getArtifactById(artifactId);
  if (!artifact) {
    return NextResponse.json({ error: "Artifact not found." }, { status: 404 });
  }

  if (artifact.storageProvider?.trim() && artifact.remotePath.trim()) {
    return NextResponse.redirect(
      new URL(`/artifacts/${artifactId}/open`, getLocalRunnerBaseUrl()),
    );
  }

  const remoteUrl = artifact.remoteUrl.trim();
  if (remoteUrl) {
    return NextResponse.redirect(remoteUrl);
  }

  return new NextResponse(artifact.contentMarkdown || "", {
    headers: {
      "content-type": "text/markdown; charset=utf-8",
      "content-disposition": `inline; filename="${artifact.title || artifact.artifactId}.md"`,
    },
  });
}
