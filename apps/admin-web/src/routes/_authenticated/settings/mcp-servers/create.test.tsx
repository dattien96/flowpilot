import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { McpCreatePage } from "./create";

const mocks = vi.hoisted(() => ({
  createGatewayBundle: vi.fn(),
  invalidate: vi.fn(),
  navigate: vi.fn(),
  useLoaderData: vi.fn(),
  useSearch: vi.fn(),
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

vi.mock("@/presentation/components/ui/badge", () => ({
  Badge: ({ children }: { children: ReactNode }) => <div>{children}</div>,
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
    createFileRoute: () => () => ({
      useLoaderData: mocks.useLoaderData,
      useSearch: mocks.useSearch,
    }),
    Link: ({ children }: { children: ReactNode }) => <>{children}</>,
  };
});

function renderSubject() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <McpCreatePage />
    </QueryClientProvider>,
  );
}

describe("McpCreatePage edit flow", () => {
  beforeEach(() => {
    mocks.invalidate.mockReset();
    mocks.navigate.mockReset();
    mocks.createGatewayBundle.mockReset();
    mocks.useSearch.mockReturnValue({ integrationId: "jira-1", provider: "jira" });
    mocks.useLoaderData.mockReturnValue({
      allIntegrations: [
        {
          id: "jira-1",
          projectId: "project-alpha",
          type: "jira",
          label: "Shared Jira",
          mcpTypeEnabled: true,
          configEncrypted: {
            workspaceUrl: "https://flowpilot899.atlassian.net",
            projectKey: "SCRUM",
            boardId: "7",
            email: "name@company.com",
          },
          status: "connected",
          lastSyncedAt: "2026-06-04T15:00:00.000Z",
          lastError: null,
          createdAt: "2026-06-04T15:00:00.000Z",
          updatedAt: "2026-06-04T15:00:00.000Z",
        },
      ],
      backends: [
        {
          key: "jira",
          providerType: "jira",
          label: "Atlassian MCP",
          transport: "remote",
          state: "installed",
          launcher: "npx",
          installed: true,
          binaryPath: null,
          command: "npx @modelcontextprotocol/server-atlassian",
          installHint: null,
          action: "verify",
          actionLabel: "Verify",
          lastCheckedAt: "2026-06-04T15:00:00.000Z",
          lastError: null,
        },
      ],
      health: {
        status: "online",
        runnerVersion: "0.1.0",
        cwd: "/workspace",
        os: "darwin",
        startedAt: "2026-06-04T15:00:00.000Z",
        baseUrl: "http://127.0.0.1:4317",
        errorMessage: null,
      },
      projects: [{ id: "project-alpha", name: "Alpha" }],
    });
    mocks.createGatewayBundle.mockReturnValue({
      integrationGateway: {
        updateIntegration: vi.fn().mockImplementation(async (id: string, patch: Record<string, unknown>) => ({
          id,
          projectId: String(patch.projectId ?? "project-alpha"),
          type: String(patch.type ?? "jira"),
          label: String(patch.label ?? "Shared Jira"),
          mcpTypeEnabled: Boolean(patch.mcpTypeEnabled ?? true),
          configEncrypted: (patch.configEncrypted as Record<string, unknown>) ?? {},
          status: String(patch.status ?? "connected"),
          lastSyncedAt: patch.lastSyncedAt ? String(patch.lastSyncedAt) : null,
          lastError: patch.lastError ? String(patch.lastError) : null,
          createdAt: "2026-06-04T15:00:00.000Z",
          updatedAt: "2026-06-04T15:00:00.000Z",
        })),
        createIntegration: vi.fn(),
      },
      localRunnerGateway: {
        triggerIntegrationConnection: vi.fn().mockResolvedValue({
          requestStatus: "accepted",
          integrationStatus: "connected",
          message: "ok",
        }),
      },
    });
  });

  it("prefills existing Jira config and reconnects with a rotated token", async () => {
    renderSubject();

    expect(screen.getByRole("heading", { name: "Edit MCP" })).toBeInTheDocument();
    expect(screen.getByDisplayValue("Shared Jira")).toBeInTheDocument();
    expect(screen.getByDisplayValue("https://flowpilot899.atlassian.net")).toBeInTheDocument();
    expect(screen.getByDisplayValue("SCRUM")).toBeInTheDocument();
    expect(screen.getByDisplayValue("7")).toBeInTheDocument();
    expect(screen.getByDisplayValue("name@company.com")).toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText("Paste the Atlassian API token"), {
      target: { value: "new-token" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save and Reconnect" }));

    await waitFor(() => {
      const gatewayBundle = mocks.createGatewayBundle.mock.results.at(-1)?.value;
      expect(gatewayBundle.integrationGateway.updateIntegration).toHaveBeenCalled();
      expect(gatewayBundle.localRunnerGateway.triggerIntegrationConnection).toHaveBeenCalledWith({
        projectId: "project-alpha",
        integrationId: "jira-1",
        providerType: "jira",
        action: "test",
        workspaceUrl: "https://flowpilot899.atlassian.net",
        projectKey: "SCRUM",
        boardId: "7",
        email: "name@company.com",
        apiToken: "new-token",
      });
    });

    expect(mocks.navigate).toHaveBeenCalledWith({ to: "/settings/mcp-servers" });
  });
});
