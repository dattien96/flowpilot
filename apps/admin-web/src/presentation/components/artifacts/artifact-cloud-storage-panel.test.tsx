// @vitest-environment jsdom
import { render, waitFor } from "@testing-library/react";
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
  supabase: {
    auth: {
      getSession,
    },
  },
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
});
