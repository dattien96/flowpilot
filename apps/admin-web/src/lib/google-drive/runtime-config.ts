import type {
  GoogleDriveArtifactSyncStatus,
  GoogleDriveMcpStatus,
  GoogleDriveWorkspaceConfigResponse,
} from "@/domain/model/entity/local-runner";

export type {
  GoogleDriveArtifactSyncStatus,
  GoogleDriveMcpStatus,
  GoogleDriveWorkspaceConfigResponse,
} from "@/domain/model/entity/local-runner";

export type GoogleDriveRuntimeStatus = GoogleDriveWorkspaceConfigResponse;

export type GoogleDriveWorkspaceConfigRequest = {
  clientId?: string;
  clientSecret?: string;
  redirectUri?: string;
  pickerApiKey?: string;
  mcpAccountId?: string;
};

export type GoogleDriveMcpOAuthUploadRequest = {
  fileName?: string;
  content?: string;
};

export type GoogleDriveValidationCheck = {
  key: string;
  status: "passed" | "failed" | "skipped";
  message: string;
};

export type GoogleDriveValidationResult = {
  valid: boolean;
  checks: GoogleDriveValidationCheck[];
  status: GoogleDriveWorkspaceConfigResponse;
};

export async function loadGoogleDriveRuntimeStatus(): Promise<GoogleDriveRuntimeStatus> {
  const response = await fetch("/api/runtime/google-drive-config", {
    cache: "no-store",
  });
  const payload = (await response.json().catch(() => null)) as GoogleDriveRuntimeStatus | null;
  if (!response.ok || !payload) {
    throw new Error((payload as { error?: string } | null)?.error ?? "Unable to load Google Drive status.");
  }
  return payload;
}
