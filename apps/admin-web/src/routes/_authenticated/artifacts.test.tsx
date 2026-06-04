// @vitest-environment jsdom
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ArtifactsPage } from "./artifacts";

const mocks = vi.hoisted(() => ({
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
  ArtifactRunBrowserPanel: () => <div />,
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
});
