export type GoogleDriveArtifactSyncStatus = {
  status: string;
  source: string;
  configured: boolean;
  clientId: string | null;
  redirectUri: string | null;
  hasClientSecret: boolean;
  hasPickerApiKey: boolean;
  missingFields: string[];
};

export type GoogleDriveMcpStatus = {
  status: string;
  configured: boolean;
  credentialPath: string | null;
  tokenPath: string | null;
  credentialFileExists: boolean;
  credentialFileValid: boolean;
  tokenFileExists: boolean;
  needsAuth: boolean;
  backendPackageAvailable: boolean;
  missingFields: string[];
};

export type GoogleDriveRuntimeStatus = {
  artifactSync: GoogleDriveArtifactSyncStatus;
  mcp: GoogleDriveMcpStatus;
  runnerReachable: boolean;
  lastError: string | null;
  updatedAt?: string;
};

export type GoogleDriveWorkspaceConfigRequest = {
  clientId?: string;
  clientSecret?: string;
  redirectUri?: string;
  pickerApiKey?: string;
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
  status: GoogleDriveRuntimeStatus;
};

export async function loadGoogleDriveRuntimeStatus() {
  const response = await fetch("/api/runtime/google-drive-config", {
    cache: "no-store",
  });
  const payload = (await response.json().catch(() => null)) as GoogleDriveRuntimeStatus | null;
  if (!response.ok || !payload) {
    throw new Error((payload as { error?: string } | null)?.error ?? "Unable to load Google Drive status.");
  }
  return payload;
}
