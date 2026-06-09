import { join } from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";

import { readGoogleDriveUploadForm, resolveEnvFallbackStatus } from "./_shared";

const envKeys = ["HOME", "USERPROFILE", "XDG_CONFIG_HOME"] as const;

function setEnv(key: (typeof envKeys)[number], value?: string) {
  if (value === undefined) {
    delete process.env[key];
    return;
  }
  process.env[key] = value;
}

describe("google drive config shared helpers", () => {
  afterEach(() => {
    for (const key of envKeys) {
      delete process.env[key];
    }
    vi.restoreAllMocks();
  });

  it("prefers XDG_CONFIG_HOME for MCP credential paths", () => {
    setEnv("HOME", "/Users/demo");
    setEnv("USERPROFILE", "/Users/demo");
    setEnv("XDG_CONFIG_HOME", "/tmp/flowpilot-xdg");
    setEnv("FLOWPILOT_GOOGLE_DRIVE_PROXY_MCP", "true");

    const status = resolveEnvFallbackStatus();

    expect(status.mcp.proxyMcpEnabled).toBe(true);
    expect(status.mcp.credentialPath).toBe(
      join("/tmp/flowpilot-xdg", "google-drive-mcp", "gcp-oauth.keys.json"),
    );
    expect(status.mcp.tokenPath).toBe(
      join("/tmp/flowpilot-xdg", "google-drive-mcp", "tokens.json"),
    );
  });

  it("reads multipart uploads without relying on the global File constructor", async () => {
    const originalFile = globalThis.File;
    // Simulate the Node route runtime where File is not globally available.
    // @ts-expect-error test-only global override
    globalThis.File = undefined;

    try {
      const uploadFile = {
        name: "gcp-oauth.keys.json",
        text: async () =>
          `{"installed":{"client_id":"client-id","client_secret":"client-secret","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token"}}`,
      } as Blob & { name: string };

      const request = {
        formData: async () => ({
          get: (key: string) => (key === "file" ? uploadFile : null),
        }),
      } as Request;

      const payload = await readGoogleDriveUploadForm(request);
      expect(payload.content).toContain(`"installed"`);
    } finally {
      globalThis.File = originalFile;
    }
  });
});
