import type { DirectoryRepository, IntegrationType, ProjectWorkspaceBinding } from "@flowpilot/client-core";

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
    { key: "projectId", label: "Project ID", required: true },
    { key: "environment", label: "Environment", required: true },
  ],
  telegram: [
    { key: "botToken", label: "Bot Token", required: true },
    { key: "channelId", label: "Channel ID", required: true },
  ],
};

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
