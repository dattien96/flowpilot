import { fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { WorkflowDefinitionsPage } from "./workflows";
import type { Project } from "@/domain/model/entity/project";
import type { Workflow } from "@/domain/model/entity/workflow-engine";

const mocks = vi.hoisted(() => ({
  createGatewayBundle: vi.fn(),
  search: {} as { projectId?: string },
}));

vi.mock("@/data/repository/browser-factory", () => ({
  createGatewayBundle: mocks.createGatewayBundle,
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
    createFileRoute: () => () => ({
      useSearch: () => mocks.search,
    }),
    useLocation: () => ({ pathname: "/workflows" }),
    Link: ({ children }: { children: ReactNode }) => <>{children}</>,
    Outlet: () => null,
  };
});

function buildWorkflow(overrides: Partial<Workflow> = {}): Workflow {
  return {
    id: "wf-1",
    projectId: null,
    name: "Beta Workflow",
    description: "Reusable flow",
    isTemplate: false,
    providerOverride: null,
    modelOverride: null,
    createdBy: "demo-user",
    createdAt: "2026-05-20T00:00:00.000Z",
    updatedAt: "2026-05-20T00:00:00.000Z",
    steps: [],
    ...overrides,
  };
}

function buildProject(overrides: Partial<Project> = {}): Project {
  return {
    id: "project-alpha",
    name: "Alpha",
    description: "Project Alpha",
    platform: "web",
    repositoryUrl: "https://example.com/repo.git",
    directoryPath: null,
    ownerId: null,
    status: "active",
    artifactStoragePreference: "supabase",
    createdBy: "demo-user",
    createdAt: "2026-05-20T00:00:00.000Z",
    updatedAt: "2026-05-20T00:00:00.000Z",
    ...overrides,
  };
}

describe("WorkflowDefinitionsPage", () => {
  beforeEach(() => {
    mocks.createGatewayBundle.mockReset();
    mocks.search = {};
  });

  it("sorts workflows by the selected order", async () => {
    mocks.createGatewayBundle.mockReturnValue({
      userFavoriteGateway: {
        listFavorites: vi.fn().mockResolvedValue([]),
      },
      projectGateway: {
        listProjects: vi.fn().mockResolvedValue([buildProject()]),
      },
      workflowEngineGateway: {
        listWorkflows: vi.fn().mockResolvedValue([
          buildWorkflow({
            id: "wf-1",
            name: "Beta Workflow",
            createdAt: "2026-05-20T00:00:00.000Z",
          }),
          buildWorkflow({
            id: "wf-2",
            name: "Alpha Workflow",
            createdAt: "2026-05-18T00:00:00.000Z",
          }),
        ]),
      },
    });

    render(<WorkflowDefinitionsPage />);

    expect(await screen.findByText("Beta Workflow")).toBeInTheDocument();

    fireEvent.change(screen.getByRole("combobox", { name: /sort by/i }), {
      target: { value: "name-asc" },
    });

    let names = screen
      .getAllByRole("heading", { level: 3 })
      .map((heading) => heading.textContent);
    expect(names).toEqual(["Alpha Workflow", "Beta Workflow"]);

    fireEvent.change(screen.getByRole("combobox", { name: /sort by/i }), {
      target: { value: "updated-desc" },
    });

    names = screen
      .getAllByRole("heading", { level: 3 })
      .map((heading) => heading.textContent);
    expect(names).toEqual(["Beta Workflow", "Alpha Workflow"]);
  });
});
