"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.providerFields = exports.integrationTypes = void 0;
exports.toErrorMessage = toErrorMessage;
exports.formatTimestamp = formatTimestamp;
exports.validateDirectoryBindingsInOrder = validateDirectoryBindingsInOrder;
exports.normalizeAtlassianSiteUrl = normalizeAtlassianSiteUrl;
exports.findDuplicateJiraIntegration = findDuplicateJiraIntegration;
exports.createEmptyConfig = createEmptyConfig;
exports.buildConfig = buildConfig;
exports.stripSecretFields = stripSecretFields;
function toErrorMessage(error, fallback) {
    return error instanceof Error ? error.message : fallback;
}
function formatTimestamp(value) {
    if (!value)
        return "Never";
    const time = new Date(value);
    return Number.isNaN(time.getTime()) ? value : time.toLocaleString();
}
async function validateDirectoryBindingsInOrder(bindings, directories) {
    if (bindings.length === 0) {
        throw new Error("Add at least one directory binding before launching project workflows.");
    }
    const results = await Promise.all(bindings.map(async (binding) => {
        try {
            return await directories.validatePath(binding.localPath);
        }
        catch (error) {
            return {
                path: binding.localPath,
                usable: false,
                reason: toErrorMessage(error, "Unable to validate path."),
            };
        }
    }));
    const usable = results.find((result) => result.usable);
    if (usable)
        return usable.path;
    throw new Error([
        "No usable project directory binding found.",
        ...results.map((result) => `- ${result.path}: ${result.reason || "unusable path"}`),
    ].join("\n"));
}
exports.integrationTypes = [
    "jira",
    "figma",
    "google_drive",
    "firebase",
    "telegram",
];
exports.providerFields = {
    jira: [
        {
            key: "workspaceUrl",
            label: "Workspace URL",
            type: "url",
            required: true,
            helpTitle: "How to get the Workspace URL",
            helpBody: "Use the base Atlassian site URL for the workspace, not a deep Jira page.",
            helpItems: [
                "Open Jira or Confluence in the browser.",
                "Copy the origin only, for example https://your-company.atlassian.net.",
                "Do not paste board paths, ticket URLs, or query parameters.",
            ],
        },
        {
            key: "projectKey",
            label: "Project Key",
            required: true,
            helpTitle: "How to get the Project Key",
            helpBody: "This is the short Jira key such as SCRUM or FLOW, not the display name.",
            helpItems: [
                "Open the Jira project.",
                "Look for the key in issue IDs like SCRUM-123.",
                "You can also find it in Project settings or the sidebar header.",
            ],
        },
        {
            key: "boardId",
            label: "Board ID",
            type: "number",
            helpTitle: "How to get the Board ID",
            helpBody: "Board ID is optional and is mainly useful for sprint-oriented flows.",
            helpItems: [
                "Open the board in Jira.",
                "Read the numeric id from the board URL.",
                "Leave this blank if you only need issue-level access.",
            ],
        },
        {
            key: "email",
            label: "Atlassian Email",
            type: "email",
            required: true,
            helpTitle: "Which email to use",
            helpBody: "Use the Atlassian account email that owns the API token and has access to the target Jira site.",
        },
        {
            key: "apiToken",
            label: "API Token",
            type: "password",
            required: true,
            helpTitle: "How to get the API token",
            helpBody: "Create an Atlassian API token with MCP access. FlowPilot sends it once to the runner keyring and does not store it in Supabase config. If Jira returns 403 Forbidden from Teamwork Graph, reconnect with a modern scoped token — legacy tokens may be rejected.",
            helpItems: [
                "Open the Atlassian API token page.",
                "Create an MCP-scoped token with the required scopes.",
                "If you see 403 Forbidden from Jira Teamwork Graph, do not reuse a legacy token; generate a modern scoped token instead.",
                "Paste the token here once for Connect and Configure Providers.",
            ],
            helpLinks: [
                { label: "Create MCP-scoped API token", href: "https://id.atlassian.com/manage-profile/security/api-tokens?autofillToken&expiryDays=max&appId=mcp&selectedScopes=all" },
                { label: "Atlassian MCP API token docs", href: "https://support.atlassian.com/atlassian-rovo-mcp-server/docs/configuring-authentication-via-api-token/" },
            ],
        },
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
        {
            key: "firebaseProjectId",
            label: "Firebase Project ID",
            required: true,
            helpTitle: "How to get the Firebase Project ID",
            helpBody: "Use the stable Firebase project id, not the display name.",
            helpItems: [
                "Open Firebase Console.",
                "Go to Project settings.",
                "Copy the Project ID value.",
            ],
        },
        {
            key: "firebaseEnvironment",
            label: "Environment",
            required: true,
            helpTitle: "What to put in Environment",
            helpBody: "Use the label that helps your team distinguish targets such as production, staging, or development.",
        },
        {
            key: "serviceAccountJson",
            label: "Service Account JSON",
            type: "password",
            required: true,
            helpTitle: "How to get the Service Account JSON",
            helpBody: "Paste the raw JSON contents of a GCP service account key that can read Crashlytics for this Firebase project.",
            helpItems: [
                "Open Google Cloud Console for the same project.",
                "Create or reuse a service account with the required Firebase or Crashlytics read permissions.",
                "Generate a JSON key and paste the full file contents here.",
            ],
        },
    ],
    telegram: [
        {
            key: "botToken",
            label: "Bot Token",
            required: true,
            helpTitle: "How to get the Bot Token",
            helpBody: "Create a Telegram bot with BotFather and copy the token it returns.",
            helpItems: [
                "In Telegram, open BotFather.",
                "Run /newbot or select an existing bot.",
                "Copy the bot token and paste it here once for Connect.",
            ],
        },
        {
            key: "channelId",
            label: "Channel ID",
            required: true,
            helpTitle: "How to get the Channel or Chat ID",
            helpBody: "Use the destination id that the bot can send to. The bot must already be a member or admin where required.",
            helpItems: [
                "Add the bot to the target channel or group.",
                "Send any message in that channel or group, then open https://api.telegram.org/bot<TOKEN>/getUpdates in a browser (replace <TOKEN> with the bot token) and read the chat.id field in the response.",
                "Or use a Telegram ID bot (e.g. forward a message from the chat to it) to read the chat id instead.",
                "Channels/groups typically use a negative numeric id like -100123456789.",
            ],
        },
    ],
};
/**
 * Normalizes a Jira/Atlassian site URL to its origin so duplicate detection
 * (Task-228 T-5, CP-05-01 §3.2) ignores path segments (e.g.
 * `/jira/software/projects/SCRUM/boards/1`), trailing slashes, and scheme
 * case. Returns "" for an unparsable/empty input so callers can treat that
 * as "nothing to compare" rather than throwing on a stray form value.
 */
function normalizeAtlassianSiteUrl(rawUrl) {
    const trimmed = rawUrl.trim();
    if (!trimmed)
        return "";
    try {
        const withScheme = /^[a-zA-Z][a-zA-Z0-9+.-]*:\/\//.test(trimmed) ? trimmed : `https://${trimmed}`;
        const parsed = new URL(withScheme);
        return parsed.origin.toLowerCase();
    }
    catch {
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
function findDuplicateJiraIntegration(integrations, projectId, candidateWorkspaceUrl) {
    const candidate = normalizeAtlassianSiteUrl(candidateWorkspaceUrl);
    if (!candidate)
        return null;
    for (const integration of integrations) {
        if (integration.type !== "jira" || integration.projectId !== projectId)
            continue;
        const existingUrl = integration.configEncrypted?.workspaceUrl;
        if (typeof existingUrl !== "string")
            continue;
        if (normalizeAtlassianSiteUrl(existingUrl) === candidate)
            return integration;
    }
    return null;
}
function createEmptyConfig(type) {
    return Object.fromEntries(exports.providerFields[type].map((field) => [field.key, ""]));
}
function buildConfig(type, values) {
    const config = {};
    for (const field of exports.providerFields[type]) {
        const value = values[field.key]?.trim() ?? "";
        if (field.required && !value) {
            throw new Error(`${field.label} is required.`);
        }
        if (value)
            config[field.key] = value;
    }
    return config;
}
/**
 * Field keys that must never be persisted into Supabase `config_encrypted`
 * (SD-11 §6 secret boundary) — the runner keyring is the only allowed store
 * for these. `buildConfig`'s output goes to the runner's
 * `/integrations/{id}/connection` endpoint (via testIntegration) where the
 * runner extracts and stores these in its own keyring; `stripSecretFields` is
 * what the Supabase-facing `createIntegration` call should send instead, so a
 * raw credential never round-trips through `config_encrypted`.
 */
const secretConfigFields = {
    jira: ["apiToken"],
    firebase: ["serviceAccountJson"],
    telegram: ["botToken"],
};
function stripSecretFields(type, config) {
    const secretKeys = secretConfigFields[type];
    if (!secretKeys || secretKeys.length === 0)
        return config;
    const stripped = { ...config };
    for (const key of secretKeys)
        delete stripped[key];
    return stripped;
}
