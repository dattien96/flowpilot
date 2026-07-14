import test from "node:test";
import assert from "node:assert/strict";
import { findDuplicateJiraIntegration, normalizeAtlassianSiteUrl, stripSecretFields } from "./settingsHelpers";

test("normalizeAtlassianSiteUrl strips path segments and trailing slash", () => {
  assert.equal(
    normalizeAtlassianSiteUrl("https://flowpilot899.atlassian.net/jira/software/projects/SCRUM/boards/1"),
    "https://flowpilot899.atlassian.net",
  );
  assert.equal(normalizeAtlassianSiteUrl("https://flowpilot899.atlassian.net/"), "https://flowpilot899.atlassian.net");
  assert.equal(normalizeAtlassianSiteUrl("https://flowpilot899.atlassian.net"), "https://flowpilot899.atlassian.net");
});

test("normalizeAtlassianSiteUrl is scheme/case-insensitive and defaults to https", () => {
  assert.equal(normalizeAtlassianSiteUrl("HTTPS://Flowpilot899.Atlassian.net"), "https://flowpilot899.atlassian.net");
  assert.equal(normalizeAtlassianSiteUrl("flowpilot899.atlassian.net"), "https://flowpilot899.atlassian.net");
});

test("normalizeAtlassianSiteUrl returns empty string for blank/unparsable input", () => {
  assert.equal(normalizeAtlassianSiteUrl(""), "");
  assert.equal(normalizeAtlassianSiteUrl("   "), "");
});

function fakeJiraIntegration(overrides: Partial<{ projectId: string | null; workspaceUrl: string }> = {}) {
  return {
    id: "int-1",
    projectId: overrides.projectId ?? "project-alpha",
    type: "jira" as const,
    label: "Jira MCP",
    mcpTypeEnabled: true,
    configEncrypted: { workspaceUrl: overrides.workspaceUrl ?? "https://flowpilot899.atlassian.net" },
    status: "connected" as const,
    lastSyncedAt: null,
    lastError: null,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  };
}

test("findDuplicateJiraIntegration matches same site with a different path", () => {
  const existing = fakeJiraIntegration({ workspaceUrl: "https://flowpilot899.atlassian.net" });
  const dup = findDuplicateJiraIntegration(
    [existing],
    "project-alpha",
    "https://flowpilot899.atlassian.net/jira/software/projects/SCRUM/boards/1",
  );
  assert.equal(dup, existing);
});

test("findDuplicateJiraIntegration ignores a different project", () => {
  const existing = fakeJiraIntegration({ projectId: "project-beta" });
  const dup = findDuplicateJiraIntegration([existing], "project-alpha", "https://flowpilot899.atlassian.net");
  assert.equal(dup, null);
});

test("findDuplicateJiraIntegration scopes workspace-global integrations separately", () => {
  const existing = fakeJiraIntegration({ projectId: null });
  const dup = findDuplicateJiraIntegration([existing], null, "https://flowpilot899.atlassian.net");
  assert.equal(dup, existing);
});

test("findDuplicateJiraIntegration ignores a different Atlassian site", () => {
  const existing = fakeJiraIntegration({ workspaceUrl: "https://other-site.atlassian.net" });
  const dup = findDuplicateJiraIntegration([existing], "project-alpha", "https://flowpilot899.atlassian.net");
  assert.equal(dup, null);
});

test("findDuplicateJiraIntegration ignores non-jira integrations", () => {
  const existing = { ...fakeJiraIntegration(), type: "google_drive" as const };
  const dup = findDuplicateJiraIntegration([existing as never], "project-alpha", "https://flowpilot899.atlassian.net");
  assert.equal(dup, null);
});

test("findDuplicateJiraIntegration returns null for an empty candidate URL", () => {
  const existing = fakeJiraIntegration();
  const dup = findDuplicateJiraIntegration([existing], "project-alpha", "");
  assert.equal(dup, null);
});

test("stripSecretFields removes serviceAccountJson for firebase", () => {
  const config = { firebaseProjectId: "flowpilot-test", firebaseEnvironment: "production", serviceAccountJson: "{secret}" };
  const stripped = stripSecretFields("firebase", config);
  assert.deepEqual(stripped, { firebaseProjectId: "flowpilot-test", firebaseEnvironment: "production" });
  // original object must not be mutated
  assert.equal(config.serviceAccountJson, "{secret}");
});

test("stripSecretFields removes botToken for telegram", () => {
  const config = { botToken: "123456:ABC", channelId: "-100123456" };
  const stripped = stripSecretFields("telegram", config);
  assert.deepEqual(stripped, { channelId: "-100123456" });
});

test("stripSecretFields removes apiToken for jira (SD-11 keyring-only)", () => {
  const config = {
    workspaceUrl: "https://x.atlassian.net",
    projectKey: "SCRUM",
    email: "name@company.com",
    apiToken: "secret-jira-token",
  };
  const stripped = stripSecretFields("jira", config);
  assert.deepEqual(stripped, {
    workspaceUrl: "https://x.atlassian.net",
    projectKey: "SCRUM",
    email: "name@company.com",
  });
  assert.equal(config.apiToken, "secret-jira-token");
});

test("stripSecretFields leaves non-secret-bearing types unchanged", () => {
  const config = { folderId: "1AbCdEf" };
  const stripped = stripSecretFields("google_drive", config);
  assert.deepEqual(stripped, config);
});
