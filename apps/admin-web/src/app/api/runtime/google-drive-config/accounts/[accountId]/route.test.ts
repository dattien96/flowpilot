import { beforeEach, describe, expect, it, vi } from "vitest";

import { DELETE } from "./route";

const mocks = vi.hoisted(() => ({
  localRunnerRequest: vi.fn(),
  readRunnerError: vi.fn(),
}));

vi.mock("../../_shared", () => ({
  localRunnerRequest: mocks.localRunnerRequest,
  readRunnerError: mocks.readRunnerError,
}));

describe("Google Drive account disconnect route", () => {
  beforeEach(() => {
    mocks.localRunnerRequest.mockReset();
    mocks.readRunnerError.mockReset();
  });

  it("forwards account disconnect requests to the runner", async () => {
    mocks.localRunnerRequest.mockResolvedValue({
      ok: true,
    });

    const response = await DELETE(
      new Request("http://localhost/api/runtime/google-drive-config/accounts/account-1", {
        method: "DELETE",
      }),
      {
        params: Promise.resolve({ accountId: "account-1" }),
      },
    );

    expect(mocks.localRunnerRequest).toHaveBeenCalledWith(
      "/google-drive-config/accounts/account-1",
      expect.objectContaining({ method: "DELETE" }),
    );
    expect(response.status).toBe(204);
  });

  it("rejects empty account ids", async () => {
    const response = await DELETE(
      new Request("http://localhost/api/runtime/google-drive-config/accounts/", {
        method: "DELETE",
      }),
      {
        params: Promise.resolve({ accountId: " " }),
      },
    );

    expect(response.status).toBe(400);
    await expect(response.json()).resolves.toEqual({ error: "accountId is required." });
  });
});
