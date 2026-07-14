import type { IntegrationType } from "@/domain/model/entity/integration";
import type { Integration } from "@/domain/model/entity/integration";

export type ProviderField = {
  key: string;
  label: string;
  placeholder: string;
  required?: boolean;
  helpText?: string;
  inputType?: "text" | "email" | "password" | "url" | "number";
  autoComplete?: string;
};

export const integrationTypes: IntegrationType[] = [
  "jira",
  "figma",
  "google_drive",
  "firebase",
  "telegram",
];

export const jiraMcpGuideLinks = [
  {
    label: "Atlassian Rovo MCP — getting started",
    href: "https://support.atlassian.com/atlassian-rovo-mcp-server/docs/getting-started-with-the-atlassian-remote-mcp-server/",
  },
  {
    label: "Enable API token auth for Rovo MCP (admin)",
    href: "https://support.atlassian.com/security-and-access-policies/docs/control-atlassian-rovo-mcp-server-settings/#Configure-authentication",
  },
  {
    label: "Configure auth via API token (Basic)",
    href: "https://support.atlassian.com/atlassian-rovo-mcp-server/docs/configuring-authentication-via-api-token/",
  },
  {
    label: "Create MCP-scoped personal API token",
    href: "https://id.atlassian.com/manage-profile/security/api-tokens?autofillToken&expiryDays=max&appId=mcp&selectedScopes=all",
  },
] as const;

export const providerFields: Record<IntegrationType, ProviderField[]> = {
  jira: [
    {
      key: "workspaceUrl",
      label: "Workspace URL",
      placeholder: "https://example.atlassian.net",
      required: true,
      inputType: "url",
      helpText: "Use the Atlassian site root, not a full board URL.",
    },
    {
      key: "projectKey",
      label: "Project Key",
      placeholder: "SCRUM",
      required: true,
      helpText: "Jira project key used by this project.",
    },
    {
      key: "boardId",
      label: "Board ID",
      placeholder: "1",
      helpText: "Optional numeric board id when workflows must target one specific board.",
      inputType: "number",
    },
    {
      key: "email",
      label: "Atlassian Email",
      placeholder: "name@company.com",
      required: true,
      inputType: "email",
      autoComplete: "email",
      helpText: "Use the account email tied to the Atlassian API token.",
    },
    {
      key: "apiToken",
      label: "API Token",
      placeholder: "Paste the Atlassian API token",
      required: true,
      inputType: "password",
      autoComplete: "new-password",
      helpText:
        "Leave blank when editing unless you want to rotate the token. The token is sent only to the local runner at connect time.",
    },
  ],
  figma: [
    {
      key: "fileKey",
      label: "File Key",
      placeholder: "AbCdEfGh123456",
      required: true,
    },
    {
      key: "teamId",
      label: "Team ID",
      placeholder: "Optional team or workspace id",
    },
  ],
  google_drive: [
    {
      key: "folderId",
      label: "Folder ID",
      placeholder: "1AbCdEfGhIjKlMn",
      required: true,
      helpText: "Google Drive folder used as the project context root.",
    },
    {
      key: "driveId",
      label: "Shared Drive ID",
      placeholder: "Optional shared drive id",
    },
  ],
  firebase: [
    {
      key: "projectId",
      label: "Project ID",
      placeholder: "flowpilot-admin",
      required: true,
    },
    {
      key: "environment",
      label: "Environment",
      placeholder: "staging",
      required: true,
    },
  ],
  telegram: [
    {
      key: "botToken",
      label: "Bot Token",
      placeholder: "123456:ABCDEF",
      required: true,
    },
    {
      key: "channelId",
      label: "Channel ID",
      placeholder: "@project_updates",
      required: true,
    },
  ],
};

export function toTitleCase(value: string) {
  return value
    .split("_")
    .map((segment) => segment.charAt(0).toUpperCase() + segment.slice(1))
    .join(" ");
}

export function createEmptyDraftConfig(type: IntegrationType) {
  return Object.fromEntries(providerFields[type].map((field) => [field.key, ""])) as Record<
    string,
    string
  >;
}

export function configToDraftValues(
  type: IntegrationType,
  config: Record<string, unknown>,
) {
  const nextValues = createEmptyDraftConfig(type);
  for (const field of providerFields[type]) {
    if (type === "jira" && field.key === "apiToken") {
      continue;
    }
    let value = config[field.key];
    if (type === "jira" && field.key === "projectKey" && typeof value !== "string") {
      value = config.board;
    }
    nextValues[field.key] = typeof value === "string" ? value : "";
  }
  return nextValues;
}

export function buildConfigFromDraft(
  type: IntegrationType,
  draftValues: Record<string, string>,
) {
  const config: Record<string, unknown> = {};
  for (const field of providerFields[type]) {
    if (type === "jira" && field.key === "apiToken") {
      continue;
    }
    const value = draftValues[field.key]?.trim() ?? "";
    if (field.required && !value) {
      throw new Error(`${field.label} is required.`);
    }
    if (value) {
      config[field.key] = value;
    }
  }
  return config;
}

export function normalizeJiraSiteUrl(value: string) {
  const trimmed = value.trim();
  if (!trimmed) {
    return "";
  }

  const candidate = /^https?:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`;

  try {
    const url = new URL(candidate);
    return `${url.protocol}//${url.host}`.toLowerCase();
  } catch {
    return trimmed.replace(/\/+$/, "").toLowerCase();
  }
}

export function findDuplicateJiraIntegration(
  integrations: Integration[],
  draftValues: Record<string, string>,
  currentIntegrationId: string | null = null,
) {
  const normalizedSiteUrl = normalizeJiraSiteUrl(draftValues.workspaceUrl ?? "");
  if (!normalizedSiteUrl) {
    return null;
  }

  return (
    integrations.find((integration) => {
      if (
        integration.type !== "jira" ||
        integration.id === currentIntegrationId ||
        integration.status !== "connected"
      ) {
        return false;
      }

      const existingSiteUrl = normalizeJiraSiteUrl(
        typeof integration.configEncrypted.workspaceUrl === "string"
          ? integration.configEncrypted.workspaceUrl
          : "",
      );
      return existingSiteUrl === normalizedSiteUrl;
    }) ?? null
  );
}

export function buildJiraConnectionRequestFields(draftValues: Record<string, string>) {
  const workspaceUrl = draftValues.workspaceUrl.trim();
  const projectKey = draftValues.projectKey.trim();
  const boardId = draftValues.boardId.trim();
  const email = draftValues.email.trim();
  const apiToken = draftValues.apiToken.trim();

  return {
    workspaceUrl: workspaceUrl || undefined,
    projectKey: projectKey || undefined,
    boardId: boardId || undefined,
    email: email || undefined,
    apiToken: apiToken || undefined,
  };
}

export function formatTimestamp(value: string | null) {
  if (!value) {
    return "Never";
  }

  return new Date(value).toLocaleString();
}
