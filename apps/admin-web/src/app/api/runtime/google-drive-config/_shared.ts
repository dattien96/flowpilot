import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

import { NextResponse } from "next/server";

import { getLocalRunnerBaseUrl } from "@/lib/env/app-env";
import type {
  GoogleDriveArtifactSyncStatus,
  GoogleDriveMcpOAuthUploadRequest,
  GoogleDriveMcpStatus,
  GoogleDriveRuntimeStatus,
  GoogleDriveWorkspaceConfigRequest,
} from "@/lib/google-drive/runtime-config";

export function localRunnerRequest(path: string, init?: RequestInit) {
  return fetch(new URL(path, getLocalRunnerBaseUrl()), {
    cache: "no-store",
    ...init,
  });
}

export async function readRunnerError(response: Response) {
  const text = await response.text();
  if (!text) {
    return `Runner request failed with status ${response.status}.`;
  }

  try {
    const payload = JSON.parse(text) as { error?: string };
    return payload.error || text;
  } catch {
    return text;
  }
}

export function browserSafeStatus(status: GoogleDriveRuntimeStatus) {
  return status;
}

export async function resolveGoogleDriveRuntimeStatus(): Promise<GoogleDriveRuntimeStatus> {
  let runnerReachable = true;
  let runnerError: string | null = null;

  try {
    const response = await localRunnerRequest("/google-drive-config");
    if (response.ok) {
      const payload = (await response.json()) as GoogleDriveRuntimeStatus;
      return browserSafeStatus(payload);
    }
    if (response.status !== 404) {
      runnerError = await readRunnerError(response);
    }
  } catch (error) {
    runnerReachable = false;
    runnerError = error instanceof Error ? error.message : "Unable to reach local runner.";
  }

  return browserSafeStatus(resolveEnvFallbackStatus(runnerReachable, runnerError));
}

export async function proxyRunnerJSON(path: string, init?: RequestInit): Promise<Response> {
  try {
    const response = await localRunnerRequest(path, init);
    const body = response.status === 204 ? null : await response.json().catch(() => null);
    if (!response.ok) {
      return NextResponse.json(
        { error: body?.error ?? `Runner request failed with status ${response.status}.` },
        { status: response.status },
      );
    }
    return NextResponse.json(body ?? {});
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Unable to reach local runner.",
      },
      { status: 400 },
    );
  }
}

export function resolveEnvFallbackStatus(
  runnerReachable = true,
  lastError: string | null = null,
): GoogleDriveRuntimeStatus {
  const artifactSync = resolveArtifactSyncEnvStatus();
  const mcp = resolveMcpEnvStatus();

  if (artifactSync.configured && mcp.status === "configured") {
    return {
      artifactSync,
      mcp,
      runnerReachable,
      lastError,
    };
  }

  return {
    artifactSync,
    mcp,
    runnerReachable,
    lastError,
  };
}

export function resolveArtifactSyncEnvStatus(): GoogleDriveArtifactSyncStatus {
  const clientId = process.env.GOOGLE_DRIVE_CLIENT_ID?.trim() || null;
  const clientSecret = process.env.GOOGLE_DRIVE_CLIENT_SECRET?.trim() || null;
  const redirectUri = normalizeRedirectUri(process.env.GOOGLE_DRIVE_REDIRECT_URI ?? "");
  const pickerApiKey = process.env.GOOGLE_PICKER_API_KEY?.trim() || null;
  const missingFields: string[] = [];
  if (!clientId) missingFields.push("clientId");
  if (!clientSecret) missingFields.push("clientSecret");
  if (!redirectUri) missingFields.push("redirectUri");
  if (!pickerApiKey) missingFields.push("pickerApiKey");

  const configured = missingFields.length === 0;
  return {
    status: configured ? "configured" : "needs_input",
    source: configured ? "env" : "missing",
    configured,
    clientId,
    redirectUri,
    hasClientSecret: Boolean(clientSecret),
    hasPickerApiKey: Boolean(pickerApiKey),
    missingFields,
  };
}

export function resolveMcpEnvStatus(): GoogleDriveMcpStatus {
  const credentialPath = getGoogleDriveCredentialPath();
  const tokenPath = getGoogleDriveTokenPath();
  const credentialFileExists = existsSync(credentialPath);
  const tokenFileExists = existsSync(tokenPath);
  const credentialFileValid = credentialFileExists ? validateOAuthJsonFile(credentialPath) : false;
  const backendPackageAvailable = true;
  const tokenFileHasRefreshToken = tokenFileExists ? hasRefreshToken(tokenPath) : false;
  const proxyMcpEnabled = isFlowPilotGoogleDriveProxyMcpEnabled();

  let status = "not_started";
  if (!credentialFileExists) {
    status = "needs_input";
  } else if (!credentialFileValid) {
    status = "failed";
  } else if (!tokenFileExists) {
    status = "needs_auth";
  } else if (!tokenFileHasRefreshToken) {
    status = "needs_auth";
  } else if (!backendPackageAvailable) {
    status = "warning";
  } else {
    status = "configured";
  }

  const missingFields: string[] = [];
  if (!credentialFileExists) missingFields.push("credentialFile");
  if (credentialFileExists && !credentialFileValid) missingFields.push("credentialJson");
  if (!tokenFileExists) missingFields.push("tokenFile");
  if (tokenFileExists && !tokenFileHasRefreshToken) missingFields.push("tokenRefresh");

  return {
    status,
    configured: status === "configured",
    proxyMcpEnabled,
    credentialPath,
    tokenPath,
    credentialFileExists,
    credentialFileValid,
    tokenFileExists,
    needsAuth: status === "needs_auth" || status === "reconnect_required",
    backendPackageAvailable,
    missingFields,
  };
}

export async function readGoogleDriveConfigForm(request: Request) {
  const contentType = request.headers.get("content-type") ?? "";
  if (contentType.includes("application/json")) {
    return (await request.json()) as GoogleDriveWorkspaceConfigRequest;
  }
  return Object.fromEntries((await request.formData()).entries()) as Record<string, FormDataEntryValue>;
}

export async function readGoogleDriveUploadForm(request: Request): Promise<GoogleDriveMcpOAuthUploadRequest> {
  const formData = await request.formData();
  const file = formData.get("file");
  if (isUploadFile(file)) {
    return {
      fileName: file.name,
      content: await file.text(),
    };
  }

  return {
    fileName: formData.get("fileName")?.toString() || undefined,
    content: formData.get("content")?.toString() || undefined,
  };
}

function isUploadFile(value: FormDataEntryValue | null): value is Blob & { name: string } {
  return Boolean(
    value &&
      typeof value === "object" &&
      typeof (value as { text?: unknown }).text === "function" &&
      typeof (value as { name?: unknown }).name === "string",
  );
}

function normalizeRedirectUri(value: string) {
  const trimmed = value.trim().replace(/\/+$/, "");
  return trimmed || null;
}

function getGoogleDriveCredentialPath() {
  return join(getGoogleDriveConfigDir(), "gcp-oauth.keys.json");
}

function getGoogleDriveTokenPath() {
  return join(getGoogleDriveConfigDir(), "tokens.json");
}

function getGoogleDriveConfigDir() {
  const xdgConfigHome = process.env.XDG_CONFIG_HOME?.trim() || "";
  if (xdgConfigHome) {
    return join(xdgConfigHome, "google-drive-mcp");
  }
  const home = process.env.HOME?.trim() || process.env.USERPROFILE?.trim() || "";
  return home ? join(home, ".config", "google-drive-mcp") : join(".", ".config", "google-drive-mcp");
}

function isFlowPilotGoogleDriveProxyMcpEnabled() {
  return process.env.FLOWPILOT_GOOGLE_DRIVE_PROXY_MCP?.trim().toLowerCase() === "true";
}

function validateOAuthJsonFile(path: string) {
  try {
    const raw = readFileSync(path, "utf8");
    const payload = JSON.parse(raw) as {
      installed?: { client_id?: string; client_secret?: string; auth_uri?: string; token_uri?: string };
      web?: { client_id?: string; client_secret?: string; auth_uri?: string; token_uri?: string };
    };
    if (!payload.installed || payload.web) {
      return false;
    }
    const source = payload.installed;
    return Boolean(
      source &&
        source.client_id &&
        source.client_secret &&
        source.auth_uri &&
        source.token_uri,
    );
  } catch {
    return false;
  }
}

function hasRefreshToken(path: string) {
  try {
    const raw = readFileSync(path, "utf8");
    const payload = JSON.parse(raw) as { refresh_token?: string };
    return Boolean(payload.refresh_token?.trim());
  } catch {
    return false;
  }
}
