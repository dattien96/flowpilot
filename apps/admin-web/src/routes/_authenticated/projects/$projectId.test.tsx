import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ProjectDetailContent } from "./$projectId";

const mocks = vi.hoisted(() => ({
  invalidate: vi.fn(),
  ensureProjectHasUsableBinding: vi.fn().mockResolvedValue(undefined),
  listStepDefinitions: vi.fn().mockResolvedValue([
    {
      stepType: "business_idea",
      name: "Business Idea",
      description: "Capture a business idea",
      promptBase: "Capture a business idea clearly.",
      requiredMcps: [],
      requiredSkills: [],
      model: "gpt-5.5",
      agentType: "standard",
      createdAt: "2026-05-20T00:00:00.000Z",
      updatedAt: "2026-05-20T00:00:00.000Z",
    },
  ]),
  listWorkflows: vi.fn().mockResolvedValue([]),
  navigate: vi.fn(),
  pathname: "/projects/project-alpha",
  startWorkflowRun: vi.fn().mockResolvedValue({
    id: "run-1",
    workflowId: "runtime-workflow",
    projectId: "project-alpha",
    status: "RUNNING",
  }),
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
    workflowEngineGateway: {
      listWorkflows: mocks.listWorkflows,
      listStepDefinitions: mocks.listStepDefinitions,
      startWorkflowRun: mocks.startWorkflowRun,
    },
    teamGateway: {
      linkTeamToProject: vi.fn(),
      unlinkTeamFromProject: vi.fn(),
    },
  }),
}));

vi.mock("@/features/projects/project-binding-launch-guard", () => ({
  ensureProjectHasUsableBinding: mocks.ensureProjectHasUsableBinding,
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
  workflowRuns: [],
  teams: [],
  allTeams: [],
  members: [],
  bindings: [],
};

async function renderSubject() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

  const rendered = render(
    <QueryClientProvider client={queryClient}>
      <ProjectDetailContent detail={detail as never} />
    </QueryClientProvider>,
  );

  await waitFor(() => {
    expect(mocks.listStepDefinitions).toHaveBeenCalled();
  });

  return rendered;
}

describe("ProjectDetailContent", () => {
  beforeEach(() => {
    mocks.ensureProjectHasUsableBinding.mockResolvedValue(undefined);
    mocks.listStepDefinitions.mockResolvedValue([
      {
        stepType: "business_idea",
        name: "Business Idea",
        description: "Capture a business idea",
        promptBase: "Capture a business idea clearly.",
        requiredMcps: [],
        requiredSkills: [],
        model: "gpt-5.5",
        agentType: "standard",
        createdAt: "2026-05-20T00:00:00.000Z",
        updatedAt: "2026-05-20T00:00:00.000Z",
      },
    ]);
    mocks.listWorkflows.mockResolvedValue([]);
    mocks.navigate.mockReset();
    mocks.startWorkflowRun.mockReset();
    mocks.startWorkflowRun.mockResolvedValue({
      id: "run-1",
      workflowId: "runtime-workflow",
      projectId: "project-alpha",
      status: "RUNNING",
    });
  });

  it("renders the workflow runs panel", async () => {
    await renderSubject();

    expect(screen.getByRole("button", { name: /open workflows/i })).toBeInTheDocument();
    expect(screen.getByText(/no workflow runs yet/i)).toBeInTheDocument();
  });

  it("keeps the edit form collapsed until expanded", async () => {
    await renderSubject();

    expect(screen.getByRole("button", { name: /expand/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /save project/i })).not.toBeInTheDocument();
  });

  it("starts single-step execution without creating a workflow definition in the browser", async () => {
    await renderSubject();

    fireEvent.click(screen.getByLabelText(/single step/i));
    fireEvent.change(screen.getByLabelText(/begin prompt/i), {
      target: { value: "Draft a new business idea." },
    });
    fireEvent.click(screen.getByRole("button", { name: /start execution/i }));

    await waitFor(() => {
      expect(mocks.startWorkflowRun).toHaveBeenCalledWith({
        projectId: "project-alpha",
        startMode: "single-step",
        beginPrompt: "Draft a new business idea.",
        stepType: "business_idea",
      });
    });
    await waitFor(() => {
      expect(mocks.navigate).toHaveBeenCalledWith({ to: "/workflow-runs" });
    });
  });
});
