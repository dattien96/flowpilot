import { beforeEach, describe, expect, it, vi } from "vitest";

import { POST } from "./route";

const mocks = vi.hoisted(() => ({
  assertAdminApiSessionForRequest: vi.fn(),
  getLocalRunnerBaseUrl: vi.fn(),
}));

vi.mock("@/data/auth/session", () => ({
  assertAdminApiSessionForRequest: mocks.assertAdminApiSessionForRequest,
}));

vi.mock("@/lib/env/app-env", () => ({
  getLocalRunnerBaseUrl: mocks.getLocalRunnerBaseUrl,
}));

describe("Google Drive artifact connect-session route", () => {
  beforeEach(() => {
    mocks.assertAdminApiSessionForRequest.mockReset();
    mocks.getLocalRunnerBaseUrl.mockReset();
    vi.unstubAllGlobals();

    mocks.assertAdminApiSessionForRequest.mockResolvedValue({
      ok: true,
      session: {
        mode: "supabase",
      },
    });
    mocks.getLocalRunnerBaseUrl.mockReturnValue("http://127.0.0.1:4317");
  });

  it("forwards the selected account id and runner base url", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      text: vi.fn().mockResolvedValue(
        JSON.stringify({
          sessionId: "session-1",
          accountId: "account-1",
          status: "awaiting_folder_selection",
          connectUrl: "http://127.0.0.1:4317/artifact-storage/google-drive/picker?sessionId=session-1",
        }),
      ),
    });
    vi.stubGlobal("fetch", fetchMock);

    const response = await POST(new Request("http://localhost/api/local-runner/artifact-storage/google-drive/connect-session", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ projectId: "project-1", accountId: "account-1" }),
    }));

    expect(fetchMock).toHaveBeenCalledWith(
      new URL("/artifact-storage/google-drive/connect-sessions", "http://127.0.0.1:4317"),
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          projectId: "project-1",
          baseUrl: "http://127.0.0.1:4317",
          accountId: "account-1",
        }),
      }),
    );
    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({
      sessionId: "session-1",
      accountId: "account-1",
      status: "awaiting_folder_selection",
      connectUrl: "http://127.0.0.1:4317/artifact-storage/google-drive/picker?sessionId=session-1",
    });
  });

  it("rejects missing project ids", async () => {
    const response = await POST(new Request("http://localhost/api/local-runner/artifact-storage/google-drive/connect-session", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ accountId: "account-1" }),
    }));

    expect(response.status).toBe(400);
    await expect(response.json()).resolves.toEqual({ error: "projectId is required." });
  });
});
