import type { DirectoryRepository, Integration, IntegrationType, ProjectWorkspaceBinding } from "@flowpilot/client-core";

export function toErrorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

export function formatTimestamp(value: string | null | undefined) {
  if (!value) return "Never";
  const time = new Date(value);
  return Number.isNaN(time.getTime()) ? value : time.toLocaleString();
}

export async function validateDirectoryBindingsInOrder(
  bindings: ProjectWorkspaceBinding[],
  directories: DirectoryRepository,
) {
  if (bindings.length === 0) {
    throw new Error("Add at least one directory binding before launching project workflows.");
  }

  const results = await Promise.all(
    bindings.map(async (binding) => {
      try {
        return await directories.validatePath(binding.localPath);
      } catch (error) {
        return {
          path: binding.localPath,
          usable: false,
          reason: toErrorMessage(error, "Unable to validate path."),
        };
      }
    }),
  );

  const usable = results.find((result) => result.usable);
  if (usable) return usable.path;

  throw new Error(
    [
      "No usable project directory binding found.",
      ...results.map((result) => `- ${result.path}: ${result.reason || "unusable path"}`),
    ].join("\n"),
  );
}

export const integrationTypes: IntegrationType[] = [
  "jira",
  "figma",
  "google_drive",
  "firebase",
  "telegram",
];

export const providerFields: Record<IntegrationType, Array<{ key: string; label: string; type?: string; required?: boolean }>> = {
  jira: [
    { key: "workspaceUrl", label: "Workspace URL", type: "url", required: true },
    { key: "projectKey", label: "Project Key", required: true },
    { key: "boardId", label: "Board ID", type: "number" },
    { key: "email", label: "Atlassian Email", type: "email", required: true },
    { key: "apiToken", label: "API Token", type: "password", required: true },
  ],
  figma: [
    { key: "fileKey", label: "File Key", required: true },
    { key: "teamId", label: "Team ID" },
  ],
  google_drive: [
    { key: "folderId", label: "Folder ID", required: true },
    { key: "driveId", label: "Shared Drive ID" },
  ],
  firebase: [
    // Task-230 (CP-05-04): field keys must match IntegrationConnectionRequest's
    // JSON tags exactly (runner.go) since McpSettings.tsx spreads
    // configEncrypted flat into the connect request body — firebaseProjectId
    // (not projectId) to avoid colliding with the request's own top-level
    // FlowPilot projectId field in the same flat JSON body.
    { key: "firebaseProjectId", label: "Firebase Project ID", required: true },
    { key: "firebaseEnvironment", label: "Environment", required: true },
    { key: "serviceAccountJson", label: "Service Account JSON", type: "password", required: true },
  ],
  telegram: [
    { key: "botToken", label: "Bot Token", required: true },
    { key: "channelId", label: "Channel ID", required: true },
  ],
};

/**
 * Normalizes a Jira/Atlassian site URL to its origin so duplicate detection
 * (Task-228 T-5, CP-05-01 §3.2) ignores path segments (e.g.
 * `/jira/software/projects/SCRUM/boards/1`), trailing slashes, and scheme
 * case. Returns "" for an unparsable/empty input so callers can treat that
 * as "nothing to compare" rather than throwing on a stray form value.
 */
export function normalizeAtlassianSiteUrl(rawUrl: string): string {
  const trimmed = rawUrl.trim();
  if (!trimmed) return "";
  try {
    const withScheme = /^[a-zA-Z][a-zA-Z0-9+.-]*:\/\//.test(trimmed) ? trimmed : `https://${trimmed}`;
    const parsed = new URL(withScheme);
    return parsed.origin.toLowerCase();
  } catch {
    return "";
  }
}

/**
 * Finds an existing Jira integration in the same project whose
 * `workspaceUrl` normalizes to the same Atlassian site as candidateUrl
 * (Task-228 T-5). Returns null when there's no conflict — including when
 * candidateUrl doesn't normalize to anything, since there's nothing to
 * compare duplicates against in that case.
 */
export function findDuplicateJiraIntegration(
  integrations: Integration[],
  projectId: string,
  candidateWorkspaceUrl: string,
): Integration | null {
  const candidate = normalizeAtlassianSiteUrl(candidateWorkspaceUrl);
  if (!candidate) return null;
  for (const integration of integrations) {
    if (integration.type !== "jira" || integration.projectId !== projectId) continue;
    const existingUrl = (integration.configEncrypted as Record<string, unknown>)?.workspaceUrl;
    if (typeof existingUrl !== "string") continue;
    if (normalizeAtlassianSiteUrl(existingUrl) === candidate) return integration;
  }
  return null;
}

export function createEmptyConfig(type: IntegrationType) {
  return Object.fromEntries(providerFields[type].map((field) => [field.key, ""])) as Record<string, string>;
}

export function buildConfig(type: IntegrationType, values: Record<string, string>) {
  const config: Record<string, unknown> = {};
  for (const field of providerFields[type]) {
    const value = values[field.key]?.trim() ?? "";
    if (field.required && !value) {
      throw new Error(`${field.label} is required.`);
    }
    if (value) config[field.key] = value;
  }
  return config;
}

/**
 * Field keys that must never be persisted into Supabase `config_encrypted`
 * (SD-11 §6 secret boundary) — the runner keyring is the only allowed store
 * for these. `buildConfig`'s output goes to the runner's `/integrations/connect`
 * (via testIntegration) where the runner extracts and stores these in its own
 * keyring; `stripSecretFields` is what the Supabase-facing `createIntegration`
 * call should send instead, so a raw credential never round-trips through
 * config_encrypted.
 */
const secretConfigFields: Partial<Record<IntegrationType, string[]>> = {
  firebase: ["serviceAccountJson"],
  telegram: ["botToken"],
};

export function stripSecretFields(type: IntegrationType, config: Record<string, unknown>): Record<string, unknown> {
  const secretKeys = secretConfigFields[type];
  if (!secretKeys || secretKeys.length === 0) return config;
  const stripped = { ...config };
  for (const key of secretKeys) delete stripped[key];
  return stripped;
}
