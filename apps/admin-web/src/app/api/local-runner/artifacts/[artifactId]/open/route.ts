import { NextResponse } from "next/server";
import path from "node:path";

import { createGatewayBundle } from "@/data/repository/factory";
import { getLocalRunnerBaseUrl } from "@/lib/env/app-env";
import { renderMarkdownPreviewHtml } from "@/lib/markdown-preview";
import { createRuntimeSupabaseAdminClient } from "@/lib/supabase/runtime-config.server";

type RouteContext = {
  params: Promise<{
    artifactId: string;
  }>;
};

export async function GET(request: Request, context: RouteContext) {
  const { artifactId } = await context.params;
  const requestUrl = new URL(request.url);
  const file = requestUrl.searchParams.get("file")?.trim() ?? "";
  const remotePath = requestUrl.searchParams.get("remotePath")?.trim() ?? "";
  const storageProvider = requestUrl.searchParams.get("storageProvider")?.trim() ?? "";
  const gateways = await createGatewayBundle();
  const artifact = await gateways.localRunnerGateway.getArtifactById(artifactId);

  const acceptHeader = request.headers.get("accept") || "";
  const isHtmlRequest = acceptHeader.includes("text/html");

  if (!artifact) {
    if (storageProvider === "supabase" && remotePath) {
      const targetPath = resolveRemoteTargetPath(remotePath, artifactId, file);
      return streamSupabaseArtifact(targetPath, isHtmlRequest);
    }
    return NextResponse.json({ error: "Artifact not found." }, { status: 404 });
  }

  if (
    artifact.storageProvider?.trim() === "supabase" &&
    artifact.remotePath.trim()
  ) {
    const targetPath = resolveRemoteTargetPath(
      artifact.remotePath,
      artifactId,
      file,
    );
    return streamSupabaseArtifact(targetPath, isHtmlRequest);
  }

  if (artifact.storageProvider?.trim() && artifact.remotePath.trim()) {
    const targetUrl = new URL(`/artifacts/${artifactId}/open`, getLocalRunnerBaseUrl());
    if (file) {
      targetUrl.searchParams.set("file", file);
    }
    return NextResponse.redirect(targetUrl);
  }

  const remoteUrl = artifact.remoteUrl.trim();
  if (remoteUrl && !file) {
    return NextResponse.redirect(remoteUrl);
  }

  const inlineContent = resolveArtifactInlineContent(artifact, file);
  const fileName = resolveArtifactFileName(artifact, file);

  const contentType = contentTypeForPath(fileName);
  const isMarkdown =
    contentType.startsWith("text/markdown") ||
    fileName.toLowerCase().endsWith(".md") ||
    fileName.toLowerCase().endsWith(".markdown") ||
    !file ||
    file === "actual-prompt" ||
    file === "prompt";

  if (isMarkdown && isHtmlRequest) {
    const html = renderMarkdownPreviewHtml(inlineContent, artifact.title || fileName);
    return new NextResponse(html, {
      headers: {
        "content-type": "text/html; charset=utf-8",
        "content-disposition": `inline; filename="${fileName}.html"`,
      },
    });
  }

  return new NextResponse(inlineContent, {
    headers: {
      "content-type": "text/markdown; charset=utf-8",
      "content-disposition": `inline; filename="${fileName}"`,
    },
  });
}

async function streamSupabaseArtifact(remotePath: string, isHtmlRequest: boolean) {
  const supabase = await createRuntimeSupabaseAdminClient();
  const bucket = supabase.storage.from("flowpilot-artifacts");
  const { data, error } = await bucket.download(remotePath.trim());
  if (error || !data) {
    throw new Error(error?.message ?? "Unable to open remote artifact.");
  }

  const fileName = path.posix.basename(remotePath.trim()) || "artifact.md";
  return buildInlineSupabaseResponse(data, fileName, isHtmlRequest);
}

function buildInlineSupabaseResponse(data: Blob, fileName: string, isHtmlRequest: boolean) {
  let contentType = data.type || "";
  if (!contentType || contentType === "application/octet-stream") {
    contentType = contentTypeForPath(fileName);
  }
  if (isTextLikeContentType(contentType)) {
    const textPromise = typeof data.text === "function"
      ? data.text()
      : data.arrayBuffer().then((buf) => Buffer.from(buf).toString("utf-8"));

    return textPromise.then((content) => {
      const isMarkdown =
        contentType.startsWith("text/markdown") ||
        fileName.toLowerCase().endsWith(".md") ||
        fileName.toLowerCase().endsWith(".markdown");

      if (isMarkdown && isHtmlRequest) {
        const html = renderMarkdownPreviewHtml(content, fileName);
        return new NextResponse(html, {
          headers: {
            "content-type": "text/html; charset=utf-8",
            "content-disposition": `inline; filename="${fileName}.html"`,
          },
        });
      }
      return new NextResponse(content, {
        headers: {
          "content-type": contentType,
          "content-disposition": `inline; filename="${fileName}"`,
        },
      });
    });
  }

  return data.arrayBuffer().then((buffer) =>
    new NextResponse(buffer, {
      headers: {
        "content-type": contentType,
        "content-disposition": `inline; filename="${fileName}"`,
      },
    }),
  );
}

function contentTypeForPath(fileName: string) {
  switch (path.extname(fileName).toLowerCase()) {
    case ".json":
      return "application/json; charset=utf-8";
    case ".md":
    case ".markdown":
      return "text/markdown; charset=utf-8";
    case ".txt":
      return "text/plain; charset=utf-8";
    case ".html":
      return "text/html; charset=utf-8";
    case ".csv":
      return "text/csv; charset=utf-8";
    case ".svg":
      return "image/svg+xml";
    default:
      return "application/octet-stream";
  }
}

function isTextLikeContentType(contentType: string) {
  const normalized = contentType.toLowerCase();
  return (
    normalized.startsWith("text/") ||
    normalized.startsWith("application/json") ||
    normalized.startsWith("image/svg+xml")
  );
}

function resolveRemoteTargetPath(remotePath: string, artifactId: string, file: string) {
  const normalized = remotePath.replaceAll("\\", "/").trim();
  if (!normalized) {
    return normalized;
  }

  switch (file) {
    case "actual-prompt":
      return canonicalToSnapshotPath(normalized, artifactId, "actual-prompt.md");
    case "prompt":
      return canonicalToSnapshotPath(normalized, artifactId, "prompt.md");
    default:
      return normalized;
  }
}

function canonicalToSnapshotPath(remotePath: string, artifactId: string, fileName: string) {
  const normalized = remotePath.replaceAll("\\", "/").trim();
  const segments = normalized.split("/").filter(Boolean);
  if (
    segments.length === 9 &&
    segments[0] === "projects" &&
    segments[2] === "runs" &&
    segments[4] === "steps" &&
    segments[6] === "artifacts" &&
    segments[7]
  ) {
    return `${segments.slice(0, 6).join("/")}/.snapshots/${segments[7]}/${fileName}`;
  }

  const trimmedArtifactId = artifactId.startsWith("remote:") ? "" : artifactId.trim();
  if (!trimmedArtifactId) {
    return normalized;
  }

  const lastSlash = normalized.lastIndexOf("/");
  if (lastSlash < 0) {
    return normalized;
  }

  return `${normalized.slice(0, lastSlash)}/.snapshots/${trimmedArtifactId}/${fileName}`;
}

function resolveArtifactInlineContent(
  artifact: {
    contentMarkdown?: string;
    promptText?: string;
    actualPromptText?: string;
  },
  file: string,
) {
  switch (file) {
    case "actual-prompt":
      return artifact.actualPromptText || artifact.promptText || "";
    case "prompt":
      return artifact.promptText || "";
    default:
      return artifact.contentMarkdown || "";
  }
}

function resolveArtifactFileName(
  artifact: {
    artifactId?: string;
    title?: string;
  },
  file: string,
) {
  switch (file) {
    case "actual-prompt":
      return "actual-prompt.md";
    case "prompt":
      return "prompt.md";
    default:
      return artifact.title || artifact.artifactId || "artifact.md";
  }
}
