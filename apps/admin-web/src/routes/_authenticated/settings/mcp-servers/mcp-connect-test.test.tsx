import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { McpConnectTestContent } from "./mcp-connect-test";
import type { Integration } from "@/domain/model/entity/integration";
import type {
  LocalRunnerHealth,
  LocalRunnerMcpBackend,
  LocalRunnerMcpTestResult,
} from "@/domain/model/entity/local-runner";
import type { Project } from "@/domain/model/entity/project";

const mocks = vi.hoisted(() => ({
  createGatewayBundle: vi.fn(),
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
    localRunnerGateway: {
      listMcpTestRuns: vi.fn().mockResolvedValue([]),
      runMcpTest: vi.fn(),
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
      <McpConnectTestContent
        allIntegrations={allIntegrations}
        backends={backends}
        health={health}
        projects={projects}
      />
    </QueryClientProvider>,
  );
}

describe("MCP connect test route", () => {
  beforeEach(() => {
    mocks.createGatewayBundle.mockReset();
    mocks.createGatewayBundle.mockReturnValue(buildGatewayMocks());
  });

  it("renders five MCP test sections and runs the Jira bug-list smoke test", async () => {
    const gatewayBundle = buildGatewayMocks();
    gatewayBundle.localRunnerGateway.runMcpTest = vi.fn().mockResolvedValue({
      status: "success",
      runId: "mcp-run-1",
      backendKey: "jira",
      providerType: "jira",
      projectId: "project-alpha",
      integrationId: "integration-jira",
      command: "jira-find-open-bugs",
      stdoutSummary: "Found 3 issues",
      stderrSummary: "",
      outputMarkdown: "- SCRUM-1\n- SCRUM-2",
      artifactPaths: [".flowpilot/mcp-tests/mcp-run-1/result.json"],
      startedAt: "2026-05-19T08:00:00.000Z",
      completedAt: "2026-05-19T08:00:02.000Z",
      errorMessage: null,
    } satisfies LocalRunnerMcpTestResult);
    mocks.createGatewayBundle.mockReturnValue(gatewayBundle);

    renderSubject();

    expect(screen.getByText("Driver")).toBeInTheDocument();
    expect(screen.getByText("Jira")).toBeInTheDocument();
    expect(screen.getByText("Tele")).toBeInTheDocument();
    expect(screen.getByText("Figma")).toBeInTheDocument();
    expect(screen.getByText("Firebase")).toBeInTheDocument();

    fireEvent.click(screen.getByText("Get a list of bugs"));

    await waitFor(() =>
      expect(screen.getAllByRole("button", { name: "Run MCP Test" })[0]).not.toBeDisabled(),
    );
    fireEvent.click(screen.getAllByRole("button", { name: "Run MCP Test" })[0]);

    await waitFor(() => {
      expect(gatewayBundle.localRunnerGateway.runMcpTest).toHaveBeenCalledWith(
        expect.objectContaining({
          backendKey: "jira",
          integrationId: "integration-jira",
          projectId: "project-alpha",
          templateKey: "jira_list_bugs",
        }),
      );
      expect(screen.getByText("Found 3 issues")).toBeInTheDocument();
      expect(screen.getByText(/jira-find-open-bugs/i)).toBeInTheDocument();
    });
  });
});
