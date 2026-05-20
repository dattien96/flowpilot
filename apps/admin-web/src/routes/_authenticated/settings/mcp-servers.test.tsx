import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { McpServersPageContent } from "./mcp-servers";
import type { Integration } from "@/domain/model/entity/integration";
import type {
  LocalRunnerHealth,
  LocalRunnerMcpBackend,
} from "@/domain/model/entity/local-runner";
import type { Project } from "@/domain/model/entity/project";

const mocks = vi.hoisted(() => ({
  createGatewayBundle: vi.fn(),
  invalidate: vi.fn(),
  navigate: vi.fn(),
}));

vi.mock("@/data/repository/browser-factory", () => ({
  createGatewayBundle: mocks.createGatewayBundle,
}));

vi.mock("@/components/common/page-frame", () => ({
  PageFrame: ({
    children,
    description,
    title,
  }: {
    children: ReactNode;
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

vi.mock("@tanstack/react-router", async () => {
  const actual = await vi.importActual<typeof import("@tanstack/react-router")>(
    "@tanstack/react-router",
  );
  return {
    ...actual,
    useRouter: () => ({
      invalidate: mocks.invalidate,
      navigate: mocks.navigate,
    }),
    createFileRoute: () => () => ({}),
    Link: ({ children, to, className, onClick, ...props }: any) => (
      <button
        className={className}
        onClick={(e) => {
          onClick?.(e);
          if (to) {
            mocks.navigate({ to });
          }
        }}
        {...props}
      >
        {children}
      </button>
    ),
  };
});

function buildHealth(overrides: Partial<LocalRunnerHealth> = {}): LocalRunnerHealth {
  return {
    status: "online",
    runnerVersion: "0.1.0",
    cwd: "/workspace",
    os: "darwin",
    startedAt: "2026-05-19T08:00:00.000Z",
    baseUrl: "http://127.0.0.1:4317",
    errorMessage: null,
    ...overrides,
  };
}

function buildBackend(overrides: Partial<LocalRunnerMcpBackend> = {}): LocalRunnerMcpBackend {
  return {
    key: "jira",
    providerType: "jira",
    label: "Atlassian MCP",
    transport: "remote",
    state: "installed",
    launcher: "remote",
    installed: true,
    binaryPath: null,
    command: "https://mcp.atlassian.com/v1/mcp/authv2",
    installHint: "Use Atlassian remote MCP.",
    action: "verify",
    actionLabel: "Verify",
    lastCheckedAt: "2026-05-19T08:00:00.000Z",
    lastError: null,
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
    createdAt: "2026-05-19T00:00:00.000Z",
    updatedAt: "2026-05-19T00:00:00.000Z",
    ...overrides,
  };
}

function buildIntegration(overrides: Partial<Integration> = {}): Integration {
  return {
    id: "integration-jira",
    projectId: "project-alpha",
    type: "jira",
    mcpTypeEnabled: true,
    label: "Alpha Jira",
    configEncrypted: {
      workspaceUrl: "https://flowpilot899.atlassian.net",
      projectKey: "SCRUM",
      email: "name@company.com",
      apiToken: "secret-token",
    },
    status: "connected",
    lastSyncedAt: "2026-05-19T08:00:00.000Z",
    lastError: null,
    createdAt: "2026-05-19T08:00:00.000Z",
    updatedAt: "2026-05-19T08:00:00.000Z",
    ...overrides,
  };
}

function buildGatewayMocks() {
  return {
    integrationGateway: {
      listAllIntegrations: vi.fn().mockResolvedValue([]),
      createIntegration: vi.fn(),
      updateIntegration: vi.fn(),
      deleteIntegration: vi.fn().mockResolvedValue(undefined),
    },
    localRunnerGateway: {
      installMcpBackend: vi.fn(),
      triggerMcpBackendAction: vi.fn(),
      triggerIntegrationConnection: vi.fn(),
      listMcpTestRuns: vi.fn().mockResolvedValue([]),
      runMcpTest: vi.fn(),
      deleteIntegrationConnection: vi.fn().mockResolvedValue(undefined),
    },
  };
}

function renderSubject({
  allIntegrations = [buildIntegration()],
  backends = [buildBackend()],
  health = buildHealth(),
  projects = [buildProject()],
}: {
  allIntegrations?: Integration[];
  backends?: LocalRunnerMcpBackend[];
  health?: LocalRunnerHealth;
  projects?: Project[];
} = {}) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <McpServersPageContent
        allIntegrations={allIntegrations}
        backends={backends}
        health={health}
        projects={projects}
      />
    </QueryClientProvider>,
  );
}

describe("MCP Servers page", () => {
  beforeEach(() => {
    mocks.createGatewayBundle.mockReset();
    mocks.invalidate.mockReset();
    mocks.navigate.mockReset();
    mocks.createGatewayBundle.mockReturnValue(buildGatewayMocks());
  });

  it("renders MCP types separately from reusable MCP instances", () => {
    renderSubject({
      allIntegrations: [
        buildIntegration(),
        buildIntegration({
          id: "integration-jira-2",
          label: "Beta Jira",
          projectId: "project-beta",
          configEncrypted: { workspaceUrl: "https://beta.atlassian.net", projectKey: "OPS" },
        }),
      ],
      backends: [
        buildBackend(),
        buildBackend({
          key: "google_drive",
          providerType: "google_drive",
          label: "Google Drive MCP",
          transport: "launcher",
          command: "npx -y @piotr-agier/google-drive-mcp --help",
        }),
      ],
    });

    expect(screen.getByText("Available Backends")).toBeInTheDocument();
    expect(screen.getByText("MCP Instances")).toBeInTheDocument();
    expect(screen.getAllByText("Alpha Jira")[0]).toBeInTheDocument();
    expect(screen.getAllByText("Beta Jira")[0]).toBeInTheDocument();
    expect(screen.getAllByText("Atlassian MCP")[0]).toBeInTheDocument();
    expect(screen.getAllByText("Google Drive MCP")[0]).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open Test Console" })).toBeInTheDocument();
  });

  it("navigates create actions to the dedicated MCP creation route", async () => {
    renderSubject({ allIntegrations: [] });

    fireEvent.click(screen.getAllByRole("button", { name: "Create MCP" })[0]);

    await waitFor(() => {
      expect(mocks.navigate).toHaveBeenCalledWith({
        to: "/settings/mcp-servers/create",
      });
    });
  });

  it("edits and removes a reusable MCP instance from the global list", async () => {
    const gatewayBundle = buildGatewayMocks();
    gatewayBundle.integrationGateway.updateIntegration = vi.fn().mockResolvedValue(
      buildIntegration({ label: "Renamed Jira" }),
    );
    gatewayBundle.localRunnerGateway.triggerIntegrationConnection = vi.fn().mockResolvedValue({
      requestStatus: "accepted",
      integrationId: "integration-jira",
      integrationStatus: "connected",
      runId: null,
      message: "Connected",
    } satisfies LocalRunnerIntegrationConnectionResult);
    mocks.createGatewayBundle.mockReturnValue(gatewayBundle);

    renderSubject();

    fireEvent.click(screen.getAllByRole("button", { name: "Edit" })[0]);
    fireEvent.change(screen.getByRole("textbox", { name: /label/i }), {
      target: { value: "Renamed Jira" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save MCP" }));

    await waitFor(() => {
      expect(gatewayBundle.integrationGateway.updateIntegration).toHaveBeenCalledWith(
        "integration-jira",
        expect.objectContaining({ label: "Renamed Jira" }),
      );
    });

    fireEvent.click(screen.getAllByRole("button", { name: "Remove" })[0]);

    await waitFor(() => {
      expect(gatewayBundle.localRunnerGateway.deleteIntegrationConnection).toHaveBeenCalledWith(
        "integration-jira",
      );
      expect(gatewayBundle.integrationGateway.deleteIntegration).toHaveBeenCalledWith(
        "integration-jira",
      );
    });
  });

  it("exposes a navigation entry to the dedicated MCP test console", () => {
    renderSubject();

    expect(screen.getByRole("button", { name: "Open Test Console" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Run MCP Test" })).not.toBeInTheDocument();
  });
});
