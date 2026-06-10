import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { WorkflowDetailPage } from "./$workflowId";
import type { Project } from "@/domain/model/entity/project";
import type {
  StepDefinition,
  Workflow,
  WorkflowRun,
  WorkflowStep,
} from "@/domain/model/entity/workflow-engine";

const mocks = vi.hoisted(() => ({
  createGatewayBundle: vi.fn(),
  navigate: vi.fn(),
  params: { workflowId: "wf-global" } as { workflowId: string },
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
      useParams: () => mocks.params,
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
    model: "gpt-5.4",
    agentType: "standard",
    createdAt: "2026-05-20T00:00:00.000Z",
    updatedAt: "2026-05-20T00:00:00.000Z",
    ...overrides,
  };
}

function buildWorkflowStep(overrides: Partial<WorkflowStep> = {}): WorkflowStep {
  return {
    id: "workflow-step-1",
    workflowId: "wf-global",
    stepType: "generate_spec",
    orderIndex: 0,
    isEnabled: true,
    requiresApproval: true,
    providerOverride: null,
    modelOverride: null,
    reasoningEffortOverride: null,
    yoloMode: null,
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
    steps: [buildWorkflowStep()],
    ...overrides,
  };
}

function buildWorkflowRun(overrides: Partial<WorkflowRun> = {}): WorkflowRun {
  return {
    id: "run-1",
    workflowId: "wf-global",
    projectId: "project-alpha",
    status: "PENDING",
    provider: null,
    model: null,
    yoloMode: false,
    startedBy: "demo-user",
    startedAt: "2026-05-20T00:00:00.000Z",
    finishedAt: null,
    errorMessage: null,
    ...overrides,
  };
}

function buildGatewayBundle() {
  return {
    projectGateway: {
      listProjects: vi.fn().mockResolvedValue([buildProject()]),
    },
    workflowEngineGateway: {
      getWorkflowDetail: vi.fn().mockResolvedValue(buildWorkflow()),
      listStepDefinitions: vi.fn().mockResolvedValue([buildStepDefinition()]),
      listSupportedModels: vi.fn().mockResolvedValue([]),
      saveWorkflow: vi.fn().mockResolvedValue(buildWorkflow()),
      startWorkflowRun: vi.fn().mockResolvedValue(buildWorkflowRun()),
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

describe("WorkflowDetailPage", () => {
  beforeEach(() => {
    mocks.createGatewayBundle.mockReset();
    mocks.navigate.mockReset();
    mocks.params = { workflowId: "wf-global" };
    mocks.search = {};
    vi.spyOn(window, "alert").mockImplementation(() => undefined);
  });

  it("keeps global workflows editable and separates launch project selection", async () => {
    const gatewayBundle = buildGatewayBundle();
    mocks.createGatewayBundle.mockReturnValue(gatewayBundle);

    renderWithQuery(<WorkflowDetailPage />);

    expect(await screen.findByDisplayValue("Workspace workflow")).toBeInTheDocument();
    expect(screen.queryByText("Run in project")).not.toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: /add step/i })).toBeEnabled();
    expect(screen.getByRole("combobox", { name: /launch in project/i })).toBeInTheDocument();
    fireEvent.change(screen.getByRole("combobox", { name: /yolo/i }), {
      target: { value: "disabled" },
    });

    fireEvent.click(screen.getByRole("button", { name: "Save workflow" }));

    await waitFor(() => {
      expect(gatewayBundle.workflowEngineGateway.saveWorkflow).toHaveBeenCalledWith(
        expect.objectContaining({
          id: "wf-global",
          projectId: null,
          modelOverride: "gpt-5.4",
          reasoningEffortOverride: "medium",
          yoloMode: false,
          steps: [
            expect.objectContaining({
              yoloMode: false,
            }),
          ],
        })
      );
    });
  });
});
