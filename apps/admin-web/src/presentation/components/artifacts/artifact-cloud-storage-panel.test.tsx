// @vitest-environment jsdom
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ButtonHTMLAttributes, ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { router, getSession } = vi.hoisted(() => ({
  router: {
    replace: vi.fn(),
  },
  getSession: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => router,
}));

vi.mock("@/data/supabase/client", () => ({
  getBrowserSupabaseClient: vi.fn(async () => ({
    auth: {
      getSession,
    },
  })),
}));

vi.mock("@/presentation/components/ui/badge", () => ({
  Badge: ({ children }: { children: ReactNode }) => <span>{children}</span>,
}));

vi.mock("@/presentation/components/ui/button", () => ({
  Button: ({ children, ...props }: ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button {...props}>{children}</button>
  ),
}));

import type { Project } from "@/domain/model/entity/project";
import { ArtifactCloudStoragePanel } from "./artifact-cloud-storage-panel";

function createGoogleDriveConnectedResponse() {
  return {
    ok: true,
    status: 200,
    json: vi.fn().mockResolvedValue({
      connection: {
        projectId: "project-1",
        status: "connected",
        folderId: "folder-1",
        folderName: "Artifacts",
      },
      session: null,
    }),
  };
}

function createGoogleDriveRuntimeReadyResponse() {
  return {
    ok: true,
    status: 200,
    json: vi.fn().mockResolvedValue({
      artifactSync: {
        status: "configured",
        source: "local",
        configured: true,
        clientId: "client-id",
        redirectUri: "http://localhost/callback",
        hasClientSecret: true,
        hasPickerApiKey: true,
        missingFields: [],
      },
      mcp: {
        status: "configured",
        configured: true,
        credentialPath: null,
        tokenPath: null,
        credentialFileExists: false,
        credentialFileValid: false,
        tokenFileExists: false,
        needsAuth: false,
        backendPackageAvailable: true,
        missingFields: [],
      },
      runnerReachable: true,
      lastError: null,
    }),
  };
}

describe("ArtifactCloudStoragePanel", () => {
  const project: Project = {
    id: "project-1",
    name: "Project One",
    description: "Project description",
    platform: "web",
    repositoryUrl: "https://example.com/repo.git",
    directoryPath: null,
    ownerId: "user-1",
    status: "active",
    artifactStoragePreference: "supabase",
    createdBy: "user-1",
    createdAt: "2026-06-02T00:00:00.000Z",
    updatedAt: "2026-06-02T00:00:00.000Z",
  };

  beforeEach(() => {
    router.replace.mockReset();
    getSession.mockReset();
    getSession.mockResolvedValue({
      data: {
        session: {
          access_token: "token-1",
        },
      },
    });
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: false,
        status: 401,
        json: vi.fn().mockResolvedValue({
          error: "Authentication required.",
        }),
      }),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("redirects to login when reading Google Drive status hits an auth error", async () => {
    render(<ArtifactCloudStoragePanel projects={[project]} />);

    await waitFor(() => {
      expect(router.replace).toHaveBeenCalledWith("/login");
    });
  });

  it("shows the Google Console setup action before Google Drive can be selected", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: vi.fn().mockResolvedValue({
            connection: {
              projectId: "project-1",
              status: "disconnected",
            },
            session: null,
          }),
        })
        .mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: vi.fn().mockResolvedValue({
            artifactSync: {
              status: "needs_input",
              source: "local",
              configured: false,
              clientId: null,
              redirectUri: null,
              hasClientSecret: false,
              hasPickerApiKey: false,
              missingFields: ["clientId"],
            },
            mcp: {
              status: "needs_input",
              configured: false,
              credentialPath: null,
              tokenPath: null,
              credentialFileExists: false,
              credentialFileValid: false,
              tokenFileExists: false,
              needsAuth: false,
              backendPackageAvailable: true,
              missingFields: [],
            },
            runnerReachable: true,
            lastError: null,
          }),
        }),
    );

    render(<ArtifactCloudStoragePanel projects={[project]} />);
    fireEvent.change(screen.getByLabelText("Provider detail"), {
      target: { value: "google_drive" },
    });

    expect(await screen.findByText("Complete Google Console setup")).toBeInTheDocument();
  });

  it("switches providers through the migration endpoint", async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("/api/runtime/google-drive-config")) {
        return Promise.resolve(createGoogleDriveRuntimeReadyResponse());
      }
      if (url.includes("/api/local-runner/artifact-storage/google-drive/status")) {
        return Promise.resolve(createGoogleDriveConnectedResponse());
      }
      if (url.includes("/api/local-runner/artifact-storage/switch-provider")) {
        return Promise.resolve({
          ok: true,
          status: 200,
          json: vi.fn().mockResolvedValue({
            status: "completed",
            sourceProvider: "supabase",
            targetProvider: "google_drive",
            totalArtifacts: 3,
            syncedCount: 2,
            skippedCount: 1,
            failedCount: 0,
            failures: [],
          }),
        });
      }
      throw new Error(`Unexpected fetch url: ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<ArtifactCloudStoragePanel projects={[project]} />);
    fireEvent.change(screen.getByLabelText("Provider detail"), {
      target: { value: "google_drive" },
    });

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Select Google Drive" })).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Select Google Drive" }));

    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(
          ([url, init]) =>
            String(url).includes("/api/local-runner/artifact-storage/switch-provider") &&
            (init as { method?: string } | undefined)?.method === "POST",
        ),
      ).toBe(true);
    });

    expect((await screen.findAllByText(/Provider switch completed/i)).length).toBeGreaterThan(0);
  });

  it("shows staged provider switch progress while the migration request is in flight", async () => {
    let resolveSwitchRequest: ((value: ResponseLike) => void) | null = null;
    type ResponseLike = {
      ok: boolean;
      status: number;
      json: ReturnType<typeof vi.fn>;
    };
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("/api/runtime/google-drive-config")) {
        return Promise.resolve(createGoogleDriveRuntimeReadyResponse());
      }
      if (url.includes("/api/local-runner/artifact-storage/google-drive/status")) {
        return Promise.resolve(createGoogleDriveConnectedResponse());
      }
      if (url.includes("/api/local-runner/artifact-storage/switch-provider")) {
        return new Promise<ResponseLike>((resolve) => {
          resolveSwitchRequest = resolve;
        });
      }
      throw new Error(`Unexpected fetch url: ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<ArtifactCloudStoragePanel projects={[project]} />);
    fireEvent.change(screen.getByLabelText("Provider detail"), {
      target: { value: "google_drive" },
    });

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Select Google Drive" })).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Select Google Drive" }));

    expect(await screen.findByText(/validating target provider/i)).toBeInTheDocument();

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 1000));
    });
    expect(await screen.findByText(/checking existing replicas/i)).toBeInTheDocument();

    await act(async () => {
      resolveSwitchRequest?.({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({
          status: "completed",
          sourceProvider: "supabase",
          targetProvider: "google_drive",
          totalArtifacts: 4,
          syncedCount: 3,
          skippedCount: 1,
          failedCount: 0,
          failures: [],
        }),
      });
    });

    await waitFor(() => {
      expect(screen.getAllByText(/Provider switch completed/i).length).toBeGreaterThan(0);
      expect(screen.getByText("Total artifacts")).toBeInTheDocument();
    });
  });
});
