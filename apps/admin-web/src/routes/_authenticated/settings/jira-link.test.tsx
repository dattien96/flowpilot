import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { JiraMcpLinkPage } from "./mcp-servers/jira-link";

const mocks = vi.hoisted(() => ({
  createGatewayBundle: vi.fn(),
  invalidate: vi.fn(),
  navigate: vi.fn(),
  useLoaderData: vi.fn(),
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
    createFileRoute: () => () => ({
      useLoaderData: mocks.useLoaderData,
    }),
    Link: ({ children, to, search, className, onClick, ...props }: any) => (
      <button
        className={className}
        onClick={(event) => {
          onClick?.(event);
          if (to) {
            mocks.navigate(search ? { to, search } : { to });
          }
        }}
        {...props}
      >
        {children}
      </button>
    ),
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
      <JiraMcpLinkPage />
    </QueryClientProvider>,
  );
}

describe("Jira MCP Link page", () => {
  beforeEach(() => {
    mocks.invalidate.mockReset();
    mocks.navigate.mockReset();
    mocks.createGatewayBundle.mockReset();
    mocks.useLoaderData.mockReturnValue({
      allIntegrations: [],
      backends: [
        {
          key: "jira",
          providerType: "jira",
          label: "Atlassian MCP",
          transport: "remote",
          state: "launcher_available",
          launcher: "npx",
          installed: false,
          binaryPath: null,
          command: "npx @modelcontextprotocol/server-atlassian",
          installHint: null,
          action: "install",
          actionLabel: "Install",
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
      projects: [
        {
          id: "project-alpha",
          name: "Alpha",
        },
      ],
    });
    mocks.createGatewayBundle.mockReturnValue({
      integrationGateway: {
        updateIntegration: vi.fn(),
      },
      localRunnerGateway: {
        installMcpBackend: vi.fn().mockResolvedValue(undefined),
        triggerMcpBackendAction: vi.fn().mockResolvedValue(undefined),
      },
    });
  });

  it("keeps the install action available without a connected Jira instance", () => {
    renderSubject();

    expect(screen.getByRole("button", { name: "Install" })).toBeEnabled();
    expect(screen.getByText(/403 Forbidden/i)).toBeInTheDocument();
    expect(screen.getByText(/modern scoped token/i)).toBeInTheDocument();
  });

  it("sends first-time enable flows to Create MCP", async () => {
    renderSubject();

    fireEvent.click(screen.getByRole("button", { name: "Enable" }));

    await waitFor(() => {
      expect(mocks.navigate).toHaveBeenCalledWith({
        to: "/settings/mcp-servers/create",
        search: { provider: "jira" },
      });
    });
  });

  it("navigates to edit flow for an existing Jira instance", async () => {
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

    renderSubject();

    fireEvent.click(screen.getByRole("button", { name: "Edit / Rotate token" }));

    await waitFor(() => {
      expect(mocks.navigate).toHaveBeenCalledWith({
        to: "/settings/mcp-servers/create",
        search: { integrationId: "jira-1", provider: "jira" },
      });
    });
  });
});
