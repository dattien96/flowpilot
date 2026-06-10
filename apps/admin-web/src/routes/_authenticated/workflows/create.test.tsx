import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { CreateWorkflowPage } from "./create";
import type { Project } from "@/domain/model/entity/project";
import type { StepDefinition, Workflow } from "@/domain/model/entity/workflow-engine";

const mocks = vi.hoisted(() => ({
  createGatewayBundle: vi.fn(),
  navigate: vi.fn(),
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
    useNavigate: () => mocks.navigate,
    Link: ({ children }: { children: ReactNode }) => <>{children}</>,
  };
});

function buildProject(overrides: Partial<Project> = {}): Project {
  return {
    id: "project-alpha",
    name: "Alpha",
    description: "Alpha project",
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

function buildStepDefinition(overrides: Partial<StepDefinition> = {}): StepDefinition {
  return {
    stepType: "generate_spec",
    name: "Generate spec",
    description: "Builds the initial spec",
    promptBase: "Generate the initial project spec.",
    requiredMcps: [],
    requiredSkills: [],
    yoloMode: true,
    model: "gpt-5.4",
    agentType: "standard",
    createdAt: "2026-05-20T00:00:00.000Z",
    updatedAt: "2026-05-20T00:00:00.000Z",
    ...overrides,
  };
}

function buildWorkflow(overrides: Partial<Workflow> = {}): Workflow {
  return {
    id: "wf-global",
    projectId: null,
    name: "Workspace workflow",
    description: "Reusable workflow",
    isTemplate: false,
    providerOverride: null,
    modelOverride: null,
    reasoningEffortOverride: null,
    yoloMode: false,
    createdBy: "demo-user",
    createdAt: "2026-05-20T00:00:00.000Z",
    updatedAt: "2026-05-20T00:00:00.000Z",
    steps: [],
    ...overrides,
  };
}

function buildGatewayBundle() {
  return {
    projectGateway: {
      listProjects: vi.fn().mockResolvedValue([buildProject()]),
    },
    workflowEngineGateway: {
      listStepDefinitions: vi.fn().mockResolvedValue([buildStepDefinition()]),
      listSupportedModels: vi.fn().mockResolvedValue([]),
      saveWorkflow: vi.fn().mockResolvedValue(buildWorkflow()),
    },
  };
}

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

function renderWithQuery(ui: ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      {ui}
    </QueryClientProvider>
  );
}

describe("CreateWorkflowPage", () => {
  beforeEach(() => {
    mocks.createGatewayBundle.mockReset();
    mocks.navigate.mockReset();
    mocks.search = {};
  });

  it("allows creating a workspace-global workflow", async () => {
    const gatewayBundle = buildGatewayBundle();
    mocks.createGatewayBundle.mockReturnValue(gatewayBundle);

    renderWithQuery(<CreateWorkflowPage />);

    expect(await screen.findByRole("option", { name: "Workspace global" })).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: /owner project/i })).toHaveValue("");

    fireEvent.click(screen.getByRole("button", { name: "Save workflow" }));

    await waitFor(() => {
      expect(gatewayBundle.workflowEngineGateway.saveWorkflow).toHaveBeenCalledWith(
        expect.objectContaining({
          projectId: null,
          modelOverride: "gpt-5.4",
          reasoningEffortOverride: "medium",
          yoloMode: false,
        })
      );
    });
    expect(mocks.navigate).toHaveBeenCalledWith({
      to: "/workflows/$workflowId",
      params: { workflowId: "wf-global" },
      search: { projectId: undefined },
    });
  });

  it("saves a step YOLO override", async () => {
    const gatewayBundle = buildGatewayBundle();
    mocks.createGatewayBundle.mockReturnValue(gatewayBundle);

    renderWithQuery(<CreateWorkflowPage />);

    expect(await screen.findByRole("button", { name: "Add step" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add step" }));
    fireEvent.change(screen.getByRole("combobox", { name: /yolo/i }), {
      target: { value: "enabled" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save workflow" }));

    await waitFor(() => {
      expect(gatewayBundle.workflowEngineGateway.saveWorkflow).toHaveBeenCalledWith(
        expect.objectContaining({
          steps: [
            expect.objectContaining({
              stepType: "generate_spec",
              yoloMode: true,
            }),
          ],
        })
      );
    });
  });
});
