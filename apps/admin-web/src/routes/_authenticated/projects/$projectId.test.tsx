import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { ProjectDetailContent } from "./$projectId";

const mocks = vi.hoisted(() => ({
  invalidate: vi.fn(),
  navigate: vi.fn(),
  pathname: "/projects/project-alpha",
}));

vi.mock("@/components/common/page-frame", () => ({
  PageFrame: ({
    children,
    description,
    title,
  }: {
    children?: ReactNode;
    description: string;
    title: string;
  }) => (
    <div>
      <h1>{title}</h1>
      <p>{description}</p>
      {children}
    </div>
  ),
}));

vi.mock("@/components/project/project-section-nav", () => ({
  ProjectSectionNav: ({ projectId }: { projectId: string }) => <div>{projectId}</div>,
}));

vi.mock("@/data/repository/browser-factory", () => ({
  createGatewayBundle: () => ({
    projectGateway: { updateProject: vi.fn(), deleteProject: vi.fn() },
    teamGateway: {
      linkTeamToProject: vi.fn(),
      unlinkTeamFromProject: vi.fn(),
    },
  }),
}));

vi.mock("@tanstack/react-router", async () => {
  const actual = await vi.importActual<typeof import("@tanstack/react-router")>(
    "@tanstack/react-router",
  );

  return {
    ...actual,
    Link: ({
      children,
      params,
      to,
    }: {
      children: ReactNode;
      params?: Record<string, string>;
      to: string;
    }) => (
      <a
        href={to
          .replace("$featureId", params?.featureId ?? "")
          .replace("$projectId", params?.projectId ?? "")}
      >
        {children}
      </a>
    ),
    Outlet: () => <div>child route</div>,
    useLocation: () => ({ pathname: mocks.pathname }),
    useNavigate: () => mocks.navigate,
    useRouter: () => ({ invalidate: mocks.invalidate }),
    createFileRoute: () => () => ({
      useLoaderData: () => detail,
    }),
  };
});

const detail = {
  project: {
    id: "project-alpha",
    name: "Alpha",
    description: "Project Alpha",
    platform: "web" as const,
    repositoryUrl: "https://example.com/repo.git",
    directoryPath: null,
    ownerId: null,
    status: "active",
    artifactStoragePreference: "supabase" as const,
    createdBy: "demo-user",
    createdAt: "2026-05-20T00:00:00.000Z",
    updatedAt: "2026-05-20T00:00:00.000Z",
  },
  features: [
    {
      id: "feature-alpha",
      projectId: "project-alpha",
      title: "Alpha feature",
      businessGoal: "Ship alpha planning",
      userProblem: "Users need a fast path to feature detail.",
      expectedFlow: "Open feature detail and start a workflow.",
      acceptanceCriteria: "Feature card navigation works.",
      priority: "high" as const,
      status: "active",
      ownerId: "demo-user",
      createdAt: "2026-05-20T00:00:00.000Z",
      updatedAt: "2026-05-20T00:00:00.000Z",
    },
  ],
  workflowRuns: [],
  teams: [],
  allTeams: [],
  members: [],
};

function renderSubject() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <ProjectDetailContent detail={detail as never} />
    </QueryClientProvider>,
  );
}

describe("ProjectDetailContent", () => {
  it("renders feature cards as navigable links", () => {
    renderSubject();

    const featureLink = screen.getByRole("link", { name: /alpha feature/i });
    expect(featureLink).toHaveAttribute("href", "/features/feature-alpha");
  });
});
