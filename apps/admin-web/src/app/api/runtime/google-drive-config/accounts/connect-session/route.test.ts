import { beforeEach, describe, expect, it, vi } from "vitest";

import { POST } from "./route";

const mocks = vi.hoisted(() => ({
  localRunnerRequest: vi.fn(),
  readRunnerError: vi.fn(),
}));

vi.mock("../../_shared", () => ({
  localRunnerRequest: mocks.localRunnerRequest,
  readRunnerError: mocks.readRunnerError,
}));

describe("Google Drive account connect-session route", () => {
  beforeEach(() => {
    mocks.localRunnerRequest.mockReset();
    mocks.readRunnerError.mockReset();
  });

  it("forwards account connect requests to the runner", async () => {
    mocks.localRunnerRequest.mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue({
        sessionId: "session-1",
        status: "pending",
        connectUrl: "http://127.0.0.1:4317/artifact-storage/google-drive/connect?sessionId=session-1",
      }),
    });

    const response = await POST(new Request("http://localhost/api/runtime/google-drive-config/accounts/connect-session", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ accountId: "account-1" }),
    }));

    expect(mocks.localRunnerRequest).toHaveBeenCalledWith(
      "/google-drive-config/accounts/connect-sessions",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ accountId: "account-1" }),
      }),
    );
    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({
      sessionId: "session-1",
      status: "pending",
      connectUrl: "http://127.0.0.1:4317/artifact-storage/google-drive/connect?sessionId=session-1",
    });
  });

  it("propagates runner errors", async () => {
    mocks.localRunnerRequest.mockResolvedValue({
      ok: false,
      status: 400,
    });
    mocks.readRunnerError.mockResolvedValue("runner rejected request");

    const response = await POST(new Request("http://localhost/api/runtime/google-drive-config/accounts/connect-session", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({}),
    }));

    expect(response.status).toBe(400);
    await expect(response.json()).resolves.toEqual({ error: "runner rejected request" });
  });
});
