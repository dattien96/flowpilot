import type { SupabaseClient } from "@supabase/supabase-js";
import path from "node:path";

import type {
  ArtifactStorageGateway,
  ArtifactStorageSyncRequest,
  ArtifactStorageSyncResult,
} from "@/domain/gateway/artifact-storage-gateway";

const BUCKET_NAME = "flowpilot-artifacts";

function ensureSegment(value: string, label: string) {
  const trimmed = value.trim();
  if (!trimmed) {
    throw new Error(`${label} is required.`);
  }
  if (trimmed.includes("/") || trimmed.includes("\\")) {
    throw new Error(`${label} cannot contain path separators.`);
  }
  if (trimmed === "." || trimmed === "..") {
    throw new Error(`${label} is invalid.`);
  }
  return trimmed;
}

function toIsoMillis(value: string | null | undefined) {
  if (!value) {
    return 0;
  }
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

function toUploadBody(bytes: Uint8Array) {
  return new Blob([bytes]);
}

function contentTypeForPath(relativePath: string) {
  switch (path.extname(relativePath).toLowerCase()) {
    case ".json":
      return "application/json; charset=utf-8";
    case ".md":
    case ".markdown":
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

function buildBasePath(request: ArtifactStorageSyncRequest) {
  const projectId = ensureSegment(request.projectId, "projectId");
  const workflowRunId = ensureSegment(request.workflowRunId, "workflowRunId");
  const workflowStepKey = ensureSegment(request.workflowStepKey, "workflowStepKey");
  return `projects/${projectId}/runs/${workflowRunId}/steps/${workflowStepKey}`;
}

function buildSnapshotPath(request: ArtifactStorageSyncRequest, relativePath: string) {
  return `${buildBasePath(request)}/.snapshots/${ensureSegment(request.artifactId, "artifactId")}/${relativePath}`;
}

function buildCanonicalPath(request: ArtifactStorageSyncRequest) {
  return `${buildBasePath(request)}/${ensureSegment(request.outputFilename, "outputFilename")}`;
}

function readSourceCreatedAt(metadata: unknown) {
  if (!metadata || typeof metadata !== "object") {
    return 0;
  }

  const value = (metadata as Record<string, unknown>).sourceCreatedAt;
  return typeof value === "string" ? toIsoMillis(value) : 0;
}

function pickCanonicalFile(request: ArtifactStorageSyncRequest) {
  const exact = request.files.find((file) => file.relativePath === request.outputFilename);
  if (exact) {
    return exact;
  }

  const byName = request.files.find(
    (file) => path.posix.basename(file.relativePath) === request.outputFilename,
  );
  if (byName) {
    return byName;
  }

  throw new Error(`Output file "${request.outputFilename}" was not found in the sync bundle.`);
}

async function uploadObject(
  bucket: ReturnType<SupabaseClient["storage"]["from"]>,
  objectPath: string,
  bytes: Uint8Array,
  metadata: Record<string, string>,
) {
  const { data, error } = await bucket.upload(objectPath, toUploadBody(bytes), {
    upsert: true,
    contentType: contentTypeForPath(objectPath),
    metadata,
  });

  if (error) {
    throw new Error(error.message);
  }

  return data;
}

export class SupabaseArtifactStorageGateway implements ArtifactStorageGateway {
  constructor(private readonly supabase: SupabaseClient) {}

  async syncSnapshot(request: ArtifactStorageSyncRequest): Promise<ArtifactStorageSyncResult> {
    if (request.files.length === 0) {
      throw new Error("Artifact sync bundle did not contain any files.");
    }

    const bucket = this.supabase.storage.from(BUCKET_NAME);
    const sourceCreatedAt = request.createdAt.trim();
    const snapshotMetadata = {
      artifactId: request.artifactId,
      projectId: request.projectId,
      workflowRunId: request.workflowRunId,
      workflowStepKey: request.workflowStepKey,
      sourceCreatedAt,
      syncRole: "snapshot",
    };

    for (const file of request.files) {
      await uploadObject(
        bucket,
        buildSnapshotPath(request, file.relativePath),
        file.bytes,
        {
          ...snapshotMetadata,
          relativePath: file.relativePath,
        },
      );
    }

    const canonicalFile = pickCanonicalFile(request);
    const canonicalPath = buildCanonicalPath(request);
    const canonicalInfo = await bucket.info(canonicalPath);
    const existingCreatedAt = readSourceCreatedAt(canonicalInfo.data?.metadata);
    const requestCreatedAt = toIsoMillis(sourceCreatedAt);

    let uploadedCanonicalId: string | undefined;
    if (!canonicalInfo.data || existingCreatedAt <= requestCreatedAt) {
      const uploadResult = await uploadObject(bucket, canonicalPath, canonicalFile.bytes, {
        ...snapshotMetadata,
        relativePath: canonicalFile.relativePath,
        syncRole: "canonical",
      });
      uploadedCanonicalId = uploadResult?.id ? String(uploadResult.id) : undefined;
    }

    const latestCanonicalInfo = await bucket.info(canonicalPath);
    const remoteObjectId =
      uploadedCanonicalId ??
      (latestCanonicalInfo.data?.id ? String(latestCanonicalInfo.data.id) : null);

    return {
      storageProvider: "supabase",
      remotePath: canonicalPath,
      remoteObjectId,
    };
  }

  async createOpenUrl(remotePath: string) {
    const bucket = this.supabase.storage.from(BUCKET_NAME);
    const { data, error } = await bucket.createSignedUrl(remotePath.trim(), 300);

    if (error) {
      throw new Error(error.message);
    }

    if (!data?.signedUrl) {
      throw new Error("Unable to create a signed URL for the artifact.");
    }

    return data.signedUrl;
  }
}
