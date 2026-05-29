import { render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Project } from "@/domain/model/entity/project";
import type { Workflow, WorkflowRun } from "@/domain/model/entity/workflow-engine";

const mocks = vi.hoisted(() => {
  const location = { pathname: "/workflow-runs" };
  return {
    createGatewayBundle: vi.fn(),
    loadWorkflowRunTitleMap: vi.fn(),
    createSupabaseBrowserClient: vi.fn(),
    location,
  };
});

vi.mock("@/data/repository/browser-factory", () => ({
  createGatewayBundle: mocks.createGatewayBundle,
}));

vi.mock("@/lib/workflow-run-title", () => ({
  loadWorkflowRunTitleMap: mocks.loadWorkflowRunTitleMap,
}));

vi.mock("@/data/datasource/supabase/client", () => ({
  createSupabaseBrowserClient: mocks.createSupabaseBrowserClient,
}));

vi.mock("@/components/common/page-frame", () => ({
  PageFrame: ({
    actions,
    children,
    description,
    title,
  }: {
    actions?: ReactNode;
    children: ReactNode;
    description: string;
    title: string;
  }) => (
    <div>
      <h1>{title}</h1>
      <p>{description}</p>
      {actions}
      {children}
    </div>
  ),
}));

vi.mock("@tanstack/react-router", async () => {
  const actual = await vi.importActual<typeof import("@tanstack/react-router")>(
    "@tanstack/react-router"
  );
  return {
    ...actual,
    createFileRoute: () => () => ({}),
    useLocation: () => mocks.location,
    Link: ({
      children,
      className,
    }: {
      children: ReactNode;
      className?: string;
    }) => <div className={className}>{children}</div>,
    Outlet: () => <div>outlet</div>,
  };
});

import { WorkflowRunHistoryPage } from "./workflow-runs";

function buildRun(overrides: Partial<WorkflowRun> = {}): WorkflowRun {
  return {
    id: "run-1",
    workflowId: "workflow-1",
    projectId: "project-1",
    status: "RUNNING",
    provider: "claude",
    model: "claude-sonnet",
    reasoningEffort: "medium",
    yoloMode: false,
    startedBy: "user-1",
    startedAt: "2026-05-29T10:00:00.000Z",
    finishedAt: null,
    errorMessage: null,
    ...overrides,
  };
}

function buildWorkflow(overrides: Partial<Workflow> = {}): Workflow {
  return {
    id: "workflow-1",
    projectId: null,
    name: "Test Workflow",
    description: "desc",
    isTemplate: false,
    providerOverride: null,
    modelOverride: null,
    reasoningEffortOverride: null,
    createdBy: "user-1",
    createdAt: "2026-05-29T10:00:00.000Z",
    updatedAt: "2026-05-29T10:00:00.000Z",
    steps: [],
    ...overrides,
  };
}

function buildProject(overrides: Partial<Project> = {}): Project {
  return {
    id: "project-1",
    name: "Test Project",
    description: "desc",
    platform: "web",
    repositoryUrl: null,
    directoryPath: null,
    ownerId: null,
    status: "active",
    artifactStoragePreference: "supabase",
    createdBy: "user-1",
    createdAt: "2026-05-29T10:00:00.000Z",
    updatedAt: "2026-05-29T10:00:00.000Z",
    ...overrides,
  };
}

describe("WorkflowRunHistoryPage", () => {
  beforeEach(() => {
    mocks.location.pathname = "/workflow-runs";
    mocks.loadWorkflowRunTitleMap.mockReset();
    mocks.loadWorkflowRunTitleMap.mockResolvedValue(new Map());
    mocks.createGatewayBundle.mockReset();
    mocks.createSupabaseBrowserClient.mockReset();

    mocks.createSupabaseBrowserClient.mockReturnValue({
      from: () => ({
        select: () => ({
          eq: () => ({
            not: vi.fn().mockResolvedValue({ data: [], error: null }),
          }),
        }),
      }),
    });
  });

  it("reloads the run list when navigating from a child run detail route back to /workflow-runs", async () => {
    const listWorkflowRuns = vi
      .fn()
      .mockResolvedValueOnce([buildRun({ status: "RUNNING" })])
      .mockResolvedValueOnce([buildRun({ status: "DONE", finishedAt: "2026-05-29T10:01:00.000Z" })]);

    mocks.createGatewayBundle.mockReturnValue({
      workflowEngineGateway: {
        listWorkflowRuns,
        listWorkflows: vi.fn().mockResolvedValue([buildWorkflow()]),
      },
      projectGateway: {
        listProjects: vi.fn().mockResolvedValue([buildProject()]),
      },
      localRunnerGateway: {
        listSessions: vi.fn().mockResolvedValue([]),
      },
      workflowGateway: {
        deleteWorkflowRuns: vi.fn(),
      },
    });

    const view = render(<WorkflowRunHistoryPage />);

    expect(await screen.findByText("RUNNING")).toBeInTheDocument();
    expect(listWorkflowRuns).toHaveBeenCalledTimes(1);

    mocks.location.pathname = "/workflow-runs/run-1";
    view.rerender(<WorkflowRunHistoryPage />);

    expect(screen.getByText("outlet")).toBeInTheDocument();
    expect(listWorkflowRuns).toHaveBeenCalledTimes(1);

    mocks.location.pathname = "/workflow-runs";
    view.rerender(<WorkflowRunHistoryPage />);

    await waitFor(() => {
      expect(listWorkflowRuns).toHaveBeenCalledTimes(2);
    });

    expect(await screen.findByText("DONE")).toBeInTheDocument();
  });
});
