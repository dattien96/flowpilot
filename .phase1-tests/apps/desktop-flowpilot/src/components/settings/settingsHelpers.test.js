"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const settingsHelpers_1 = require("./settingsHelpers");
(0, node_test_1.default)("normalizeAtlassianSiteUrl strips path segments and trailing slash", () => {
    strict_1.default.equal((0, settingsHelpers_1.normalizeAtlassianSiteUrl)("https://flowpilot899.atlassian.net/jira/software/projects/SCRUM/boards/1"), "https://flowpilot899.atlassian.net");
    strict_1.default.equal((0, settingsHelpers_1.normalizeAtlassianSiteUrl)("https://flowpilot899.atlassian.net/"), "https://flowpilot899.atlassian.net");
    strict_1.default.equal((0, settingsHelpers_1.normalizeAtlassianSiteUrl)("https://flowpilot899.atlassian.net"), "https://flowpilot899.atlassian.net");
});
(0, node_test_1.default)("normalizeAtlassianSiteUrl is scheme/case-insensitive and defaults to https", () => {
    strict_1.default.equal((0, settingsHelpers_1.normalizeAtlassianSiteUrl)("HTTPS://Flowpilot899.Atlassian.net"), "https://flowpilot899.atlassian.net");
    strict_1.default.equal((0, settingsHelpers_1.normalizeAtlassianSiteUrl)("flowpilot899.atlassian.net"), "https://flowpilot899.atlassian.net");
});
(0, node_test_1.default)("normalizeAtlassianSiteUrl returns empty string for blank/unparsable input", () => {
    strict_1.default.equal((0, settingsHelpers_1.normalizeAtlassianSiteUrl)(""), "");
    strict_1.default.equal((0, settingsHelpers_1.normalizeAtlassianSiteUrl)("   "), "");
});
function fakeJiraIntegration(overrides = {}) {
    return {
        id: "int-1",
        projectId: overrides.projectId ?? "project-alpha",
        type: "jira",
        label: "Jira MCP",
        mcpTypeEnabled: true,
        configEncrypted: { workspaceUrl: overrides.workspaceUrl ?? "https://flowpilot899.atlassian.net" },
        status: "connected",
        lastSyncedAt: null,
        lastError: null,
        createdAt: "2026-01-01T00:00:00Z",
        updatedAt: "2026-01-01T00:00:00Z",
    };
}
(0, node_test_1.default)("findDuplicateJiraIntegration matches same site with a different path", () => {
    const existing = fakeJiraIntegration({ workspaceUrl: "https://flowpilot899.atlassian.net" });
    const dup = (0, settingsHelpers_1.findDuplicateJiraIntegration)([existing], "project-alpha", "https://flowpilot899.atlassian.net/jira/software/projects/SCRUM/boards/1");
    strict_1.default.equal(dup, existing);
});
(0, node_test_1.default)("findDuplicateJiraIntegration ignores a different project", () => {
    const existing = fakeJiraIntegration({ projectId: "project-beta" });
    const dup = (0, settingsHelpers_1.findDuplicateJiraIntegration)([existing], "project-alpha", "https://flowpilot899.atlassian.net");
    strict_1.default.equal(dup, null);
});
(0, node_test_1.default)("findDuplicateJiraIntegration scopes workspace-global integrations separately", () => {
    const existing = fakeJiraIntegration({ projectId: null });
    const dup = (0, settingsHelpers_1.findDuplicateJiraIntegration)([existing], null, "https://flowpilot899.atlassian.net");
    strict_1.default.equal(dup, existing);
});
(0, node_test_1.default)("findDuplicateJiraIntegration ignores a different Atlassian site", () => {
    const existing = fakeJiraIntegration({ workspaceUrl: "https://other-site.atlassian.net" });
    const dup = (0, settingsHelpers_1.findDuplicateJiraIntegration)([existing], "project-alpha", "https://flowpilot899.atlassian.net");
    strict_1.default.equal(dup, null);
});
(0, node_test_1.default)("findDuplicateJiraIntegration ignores non-jira integrations", () => {
    const existing = { ...fakeJiraIntegration(), type: "google_drive" };
    const dup = (0, settingsHelpers_1.findDuplicateJiraIntegration)([existing], "project-alpha", "https://flowpilot899.atlassian.net");
    strict_1.default.equal(dup, null);
});
(0, node_test_1.default)("findDuplicateJiraIntegration returns null for an empty candidate URL", () => {
    const existing = fakeJiraIntegration();
    const dup = (0, settingsHelpers_1.findDuplicateJiraIntegration)([existing], "project-alpha", "");
    strict_1.default.equal(dup, null);
});
(0, node_test_1.default)("stripSecretFields removes serviceAccountJson for firebase", () => {
    const config = { firebaseProjectId: "flowpilot-test", firebaseEnvironment: "production", serviceAccountJson: "{secret}" };
    const stripped = (0, settingsHelpers_1.stripSecretFields)("firebase", config);
    strict_1.default.deepEqual(stripped, { firebaseProjectId: "flowpilot-test", firebaseEnvironment: "production" });
    // original object must not be mutated
    strict_1.default.equal(config.serviceAccountJson, "{secret}");
});
(0, node_test_1.default)("stripSecretFields removes botToken for telegram", () => {
    const config = { botToken: "123456:ABC", channelId: "-100123456" };
    const stripped = (0, settingsHelpers_1.stripSecretFields)("telegram", config);
    strict_1.default.deepEqual(stripped, { channelId: "-100123456" });
});
(0, node_test_1.default)("stripSecretFields removes apiToken for jira (SD-11 keyring-only)", () => {
    const config = {
        workspaceUrl: "https://x.atlassian.net",
        projectKey: "SCRUM",
        email: "name@company.com",
        apiToken: "secret-jira-token",
    };
    const stripped = (0, settingsHelpers_1.stripSecretFields)("jira", config);
    strict_1.default.deepEqual(stripped, {
        workspaceUrl: "https://x.atlassian.net",
        projectKey: "SCRUM",
        email: "name@company.com",
    });
    strict_1.default.equal(config.apiToken, "secret-jira-token");
});
(0, node_test_1.default)("stripSecretFields leaves non-secret-bearing types unchanged", () => {
    const config = { folderId: "1AbCdEf" };
    const stripped = (0, settingsHelpers_1.stripSecretFields)("google_drive", config);
    strict_1.default.deepEqual(stripped, config);
});
