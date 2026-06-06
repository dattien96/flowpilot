// @vitest-environment jsdom
import { render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ArtifactsPage } from "./artifacts";

const mocks = vi.hoisted(() => ({
  artifactRunBrowserPanel: vi.fn(),
  createGatewayBundle: vi.fn(),
  pathname: "/artifacts",
  search: { tab: "generated" as "generated" | "storage" | "catalog" },
  navigate: vi.fn(),
}));

vi.mock("@/data/repository/browser-factory", () => ({
  createGatewayBundle: mocks.createGatewayBundle,
}));

vi.mock("@/domain/usecase/projects/list-projects-usecase", () => ({
  ListProjectsUseCase: class {
    constructor(private readonly gateway: { listProjects: () => Promise<unknown[]> }) {}
    execute() {
      return this.gateway.listProjects();
    }
  },
}));

vi.mock("@/domain/usecase/workflow-engine/list-artifact-definitions-usecase", () => ({
  ListArtifactDefinitionsUseCase: class {
    constructor(private readonly gateway: { listArtifactDefinitions: () => Promise<unknown[]> }) {}
    execute() {
      return this.gateway.listArtifactDefinitions();
    }
  },
}));

vi.mock("@/domain/usecase/workflow-engine/list-artifact-runs-usecase", () => ({
  ListArtifactRunsUseCase: class {
    constructor(private readonly gateway: { listArtifactRuns: () => Promise<unknown[]> }) {}
    execute() {
      return this.gateway.listArtifactRuns();
    }
  },
}));

vi.mock("@/domain/usecase/workflow-engine/list-workflow-runs-usecase", () => ({
  ListWorkflowRunsUseCase: class {
    constructor(private readonly gateway: { listWorkflowRuns: () => Promise<unknown[]> }) {}
    execute() {
      return this.gateway.listWorkflowRuns();
    }
  },
}));

vi.mock("@/domain/usecase/workflow-engine/save-artifact-definition-usecase", () => ({
  SaveArtifactDefinitionUseCase: class {
    constructor(private readonly gateway: { saveArtifactDefinition: (definition: unknown) => Promise<unknown> }) {}
    execute(definition: unknown) {
      return this.gateway.saveArtifactDefinition(definition);
    }
  },
}));

vi.mock("@/components/common/page-frame", () => ({
  PageFrame: ({
    actions,
    children,
    description,
    title,
  }: {
    actions?: ReactNode;
    children?: ReactNode;
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

vi.mock("@/components/ui/button", () => ({
  Button: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("@/presentation/components/artifacts/artifact-cloud-storage-panel", () => ({
  ArtifactCloudStoragePanel: () => <div>storage panel</div>,
}));

vi.mock("@/presentation/components/artifacts/artifact-run-browser-panel", () => ({
  ArtifactRunBrowserPanel: (props: unknown) => {
    mocks.artifactRunBrowserPanel(props);
    return <div>generated panel</div>;
  },
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
    useLocation: () => ({ pathname: mocks.pathname }),
    useNavigate: () => mocks.navigate,
    Link: ({ children }: { children: ReactNode }) => <>{children}</>,
    Outlet: () => <div>child route</div>,
  };
});

describe("ArtifactsPage", () => {
  beforeEach(() => {
    mocks.artifactRunBrowserPanel.mockReset();
    mocks.createGatewayBundle.mockReset();
    mocks.pathname = "/artifacts";
    mocks.search = { tab: "generated" };
    mocks.navigate.mockReset();
    mocks.createGatewayBundle.mockReturnValue({
      workflowEngineGateway: {
        listArtifactDefinitions: vi.fn().mockImplementation(() => new Promise(() => undefined)),
        listArtifactRuns: vi.fn().mockResolvedValue([]),
        listWorkflowRuns: vi.fn().mockResolvedValue([]),
        saveArtifactDefinition: vi.fn(),
      },
      projectGateway: {
        listProjects: vi.fn().mockResolvedValue([]),
      },
      localRunnerGateway: {
        listArtifacts: vi.fn().mockResolvedValue([]),
        getStorageDriver: vi.fn().mockImplementation(() => new Promise(() => undefined)),
      },
    });
  });

  it("renders the child route for nested artifact paths", () => {
    mocks.pathname = "/artifacts/create";

    render(<ArtifactsPage />);

    expect(screen.getByText("child route")).toBeInTheDocument();
    expect(screen.queryByText("Definition catalog")).not.toBeInTheDocument();
  });

  it("opens the storage tab from the search param", async () => {
    mocks.search = { tab: "storage" };
    mocks.createGatewayBundle.mockReturnValue({
      workflowEngineGateway: {
        listArtifactDefinitions: vi.fn().mockResolvedValue([]),
        listArtifactRuns: vi.fn().mockResolvedValue([]),
        listWorkflowRuns: vi.fn().mockResolvedValue([]),
        saveArtifactDefinition: vi.fn(),
      },
      projectGateway: {
        listProjects: vi.fn().mockResolvedValue([
          {
            id: "project-1",
            name: "Project One",
          },
        ]),
      },
      localRunnerGateway: {
        listArtifacts: vi.fn().mockResolvedValue([]),
        getStorageDriver: vi.fn().mockImplementation(() => new Promise(() => undefined)),
      },
    });

    render(<ArtifactsPage />);

    expect(await screen.findByText("storage panel")).toBeInTheDocument();
  });

  it("keeps valid local artifacts visible when workflow runs are missing", async () => {
    mocks.createGatewayBundle.mockReturnValue({
      workflowEngineGateway: {
        listArtifactDefinitions: vi.fn().mockResolvedValue([]),
        listArtifactRuns: vi.fn().mockResolvedValue([]),
        listWorkflowRuns: vi.fn().mockResolvedValue([]),
        saveArtifactDefinition: vi.fn(),
      },
      projectGateway: {
        listProjects: vi.fn().mockResolvedValue([
          {
            id: "project-mplan",
            name: "Mplan",
          },
        ]),
      },
      localRunnerGateway: {
        listArtifacts: vi.fn().mockResolvedValue([
          {
            artifactId: "artifact-1",
            title: "Codex Output",
            sourceKind: "workflow_output",
            projectId: "project-mplan",
            workflowRunId: "run-missing-from-db",
            workflowStepKey: "codex_test",
            providerKey: "codex",
            localPath:
              ".flowpilot/artifacts/project-mplan/run-missing-from-db/codex_test/output.md",
            remotePath: "",
            remoteUrl: "",
            storageProvider: null,
            remoteObjectId: null,
            syncStatus: "local_only",
            createdAt: "2026-06-05T00:00:00.000Z",
            updatedAt: "2026-06-05T00:00:00.000Z",
            contentMarkdown: "artifact body",
            previewMarkdown: "artifact body",
          },
          {
            artifactId: "artifact-invalid",
            title: "Invalid Output",
            sourceKind: "workflow_output",
            projectId: "",
            workflowRunId: "run-invalid",
            workflowStepKey: "codex_test",
            providerKey: "codex",
            localPath: ".flowpilot/artifacts/invalid.md",
            remotePath: "",
            remoteUrl: "",
            storageProvider: null,
            remoteObjectId: null,
            syncStatus: "local_only",
            createdAt: "2026-06-05T00:00:00.000Z",
            updatedAt: "2026-06-05T00:00:00.000Z",
            contentMarkdown: "invalid",
            previewMarkdown: "invalid",
          },
        ]),
        getStorageDriver: vi.fn().mockImplementation(() => new Promise(() => undefined)),
      },
    });

    render(<ArtifactsPage />);

    expect(await screen.findByText("generated panel")).toBeInTheDocument();
    await waitFor(() => expect(mocks.artifactRunBrowserPanel).toHaveBeenCalled());

    const lastProps = mocks.artifactRunBrowserPanel.mock.calls.at(-1)?.[0] as {
      localArtifacts: Array<{ artifactId: string }>;
    };

    expect(lastProps.localArtifacts).toEqual([
      expect.objectContaining({
        artifactId: "artifact-1",
      }),
    ]);
  });

  it("filters remote artifact rows to active workflow runs on first load", async () => {
    mocks.createGatewayBundle.mockReturnValue({
      workflowEngineGateway: {
        listArtifactDefinitions: vi.fn().mockResolvedValue([]),
        listArtifactRuns: vi.fn().mockResolvedValue([
          {
            id: "active-artifact",
            workflowRunId: "active-run",
            projectId: "project-1",
          },
          {
            id: "orphan-storage-artifact",
            workflowRunId: "deleted-run",
            projectId: "project-1",
          },
        ]),
        listWorkflowRuns: vi.fn().mockResolvedValue([
          {
            id: "active-run",
          },
        ]),
        saveArtifactDefinition: vi.fn(),
      },
      projectGateway: {
        listProjects: vi.fn().mockResolvedValue([
          {
            id: "project-1",
            name: "Project One",
          },
        ]),
      },
      localRunnerGateway: {
        listArtifacts: vi.fn().mockResolvedValue([]),
        getStorageDriver: vi.fn().mockImplementation(() => new Promise(() => undefined)),
      },
    });

    render(<ArtifactsPage />);

    expect(await screen.findByText("generated panel")).toBeInTheDocument();
    await waitFor(() => expect(mocks.artifactRunBrowserPanel).toHaveBeenCalled());

    const lastProps = mocks.artifactRunBrowserPanel.mock.calls.at(-1)?.[0] as {
      artifactRuns: Array<{ id: string }>;
    };

    expect(lastProps.artifactRuns).toEqual([
      expect.objectContaining({
        id: "active-artifact",
      }),
    ]);
  });
});
