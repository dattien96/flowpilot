import { beforeEach, describe, expect, it, vi } from "vitest";

import { GET } from "./route";

const mocks = vi.hoisted(() => ({
  assertAdminApiSessionForRequest: vi.fn(),
  createRuntimeSupabaseAdminClient: vi.fn(),
  getLocalRunnerBaseUrl: vi.fn(),
  upsertArtifactStorageConnection: vi.fn(),
}));

vi.mock("@/data/auth/session", () => ({
  assertAdminApiSessionForRequest: mocks.assertAdminApiSessionForRequest,
}));

vi.mock("@/features/artifacts/artifact-storage-connection", () => ({
  upsertArtifactStorageConnection: mocks.upsertArtifactStorageConnection,
}));

vi.mock("@/lib/env/app-env", () => ({
  getLocalRunnerBaseUrl: mocks.getLocalRunnerBaseUrl,
}));

vi.mock("@/lib/supabase/runtime-config.server", () => ({
  createRuntimeSupabaseAdminClient: mocks.createRuntimeSupabaseAdminClient,
}));

describe("Google Drive artifact storage status route", () => {
  beforeEach(() => {
    mocks.assertAdminApiSessionForRequest.mockReset();
    mocks.createRuntimeSupabaseAdminClient.mockReset();
    mocks.getLocalRunnerBaseUrl.mockReset();
    mocks.upsertArtifactStorageConnection.mockReset();
    vi.unstubAllGlobals();

    mocks.assertAdminApiSessionForRequest.mockResolvedValue({
      ok: true,
      session: {
        mode: "supabase",
        user: {
          id: "user-1",
          email: "user@example.com",
        },
      },
    });
    mocks.createRuntimeSupabaseAdminClient.mockResolvedValue({ admin: true });
    mocks.getLocalRunnerBaseUrl.mockReturnValue("http://127.0.0.1:4317");
  });

  it("syncs connected runner state into Supabase through the runtime admin client", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        text: vi.fn().mockResolvedValue(
          JSON.stringify({
            connection: {
              projectId: "project-1",
              status: "connected",
              folderId: "folder-1",
              folderName: "Artifacts",
              accountEmail: "drive@example.com",
              lastValidatedAt: "2026-06-06T15:00:00.000Z",
              connectedAt: "2026-06-06T15:00:00.000Z",
            },
            session: null,
          }),
        ),
      }),
    );

    const response = await GET(
      new Request("http://localhost/api/local-runner/artifact-storage/google-drive/status?projectId=project-1"),
    );

    expect(response.status).toBe(200);
    expect(mocks.createRuntimeSupabaseAdminClient).toHaveBeenCalled();
    expect(mocks.upsertArtifactStorageConnection).toHaveBeenCalledWith(
      { admin: true },
      expect.objectContaining({
        projectId: "project-1",
        provider: "google_drive",
        status: "connected",
        folderId: "folder-1",
        folderName: "Artifacts",
        oauthAccountEmail: "drive@example.com",
      }),
    );
  });

  it("fails connected status reads when the connection metadata cannot be persisted", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        text: vi.fn().mockResolvedValue(
          JSON.stringify({
            connection: {
              projectId: "project-1",
              status: "connected",
              folderId: "folder-1",
            },
            session: null,
          }),
        ),
      }),
    );
    mocks.upsertArtifactStorageConnection.mockRejectedValue(new Error("RLS denied"));

    const response = await GET(
      new Request("http://localhost/api/local-runner/artifact-storage/google-drive/status?projectId=project-1"),
    );

    expect(response.status).toBe(400);
    await expect(response.json()).resolves.toEqual({
      error: "RLS denied",
    });
  });
});
