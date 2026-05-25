import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ProjectSettingsContent } from "./settings";
import type { Integration } from "@/domain/model/entity/integration";

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

vi.mock("@/components/project/project-section-nav", () => ({
  ProjectSectionNav: ({ projectId }: { projectId: string }) => <div>{projectId}</div>,
}));

vi.mock("@tanstack/react-router", async () => {
  const actual = await vi.importActual<typeof import("@tanstack/react-router")>(
    "@tanstack/react-router",
  );
  return {
    ...actual,
    Link: ({ children }: { children: ReactNode }) => <>{children}</>,
    useRouter: () => ({
      invalidate: mocks.invalidate,
      navigate: mocks.navigate,
    }),
  };
});

function buildProject() {
  return {
    id: "project-alpha",
    name: "Alpha",
    description: "Project Alpha",
    platform: "web" as const,
    repositoryUrl: "https://example.com/repo.git",
    directoryPath: null,
    ownerId: null,
    status: "active",
    artifactStoragePreference: "supabase" as const,
    defaultProvider: "codex",
    defaultModel: "gpt-5.4",
    defaultReasoningEffort: "medium",
    createdBy: "demo-user",
    createdAt: "2026-05-19T00:00:00.000Z",
    updatedAt: "2026-05-19T00:00:00.000Z",
  };
}

function buildIntegration(overrides: Partial<Integration> = {}): Integration {
  return {
    id: "integration-1",
    projectId: "project-owner",
    type: "jira",
    label: "Shared Jira",
    configEncrypted: { workspaceUrl: "https://flowpilot899.atlassian.net", projectKey: "SCRUM" },
    status: "connected",
    lastSyncedAt: "2026-05-19T08:00:00.000Z",
    lastError: null,
    createdAt: "2026-05-19T08:00:00.000Z",
    updatedAt: "2026-05-19T08:00:00.000Z",
    ...overrides,
  };
}

function renderSubject(
  props: Partial<Parameters<typeof ProjectSettingsContent>[0]> = {},
) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <ProjectSettingsContent
        allTeams={[]}
        availableIntegrations={[]}
        linkedIntegrations={[]}
        project={buildProject()}
        projectId="project-alpha"
        teams={[]}
        {...props}
      />
    </QueryClientProvider>,
  );
}

describe("Project settings MCP links", () => {
  beforeEach(() => {
    mocks.createGatewayBundle.mockReset();
    mocks.invalidate.mockReset();
    mocks.navigate.mockReset();
  });

  it("shows linked MCP instances and lets the user unlink them", async () => {
    const integrationGateway = {
      linkIntegrationToProject: vi.fn(),
      unlinkIntegrationFromProject: vi.fn().mockResolvedValue(undefined),
    };
    mocks.createGatewayBundle.mockReturnValue({
      integrationGateway,
      projectGateway: { updateProject: vi.fn() },
      teamGateway: {
        linkTeamToProject: vi.fn(),
        unlinkTeamFromProject: vi.fn(),
      },
    });

    renderSubject({
      linkedIntegrations: [buildIntegration()],
      availableIntegrations: [buildIntegration()],
    });

    expect(screen.getByText("Project MCP Links")).toBeInTheDocument();
    expect(screen.getByText(/Linked to Shared Jira/i)).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Link MCP" })).toHaveLength(4);
    expect(screen.getAllByRole("button", { name: "Unlink" })).toHaveLength(1);

    fireEvent.click(screen.getAllByRole("button", { name: "Unlink" })[0]);

    await waitFor(() => {
      expect(integrationGateway.unlinkIntegrationFromProject).toHaveBeenCalledWith(
        "project-alpha",
        "integration-1",
      );
      expect(mocks.invalidate).toHaveBeenCalled();
    });
  });

  it("links an existing MCP instance instead of editing provider config locally", async () => {
    const integrationGateway = {
      linkIntegrationToProject: vi.fn().mockResolvedValue(undefined),
      unlinkIntegrationFromProject: vi.fn(),
    };
    mocks.createGatewayBundle.mockReturnValue({
      integrationGateway,
      projectGateway: { updateProject: vi.fn() },
      teamGateway: {
        linkTeamToProject: vi.fn(),
        unlinkTeamFromProject: vi.fn(),
      },
    });

    renderSubject({
      availableIntegrations: [
        buildIntegration({ id: "integration-jira", label: "Shared Jira" }),
        buildIntegration({
          id: "integration-drive",
          type: "google_drive",
          label: "Drive Folder",
          status: "awaiting_oauth",
        }),
      ],
    });

    fireEvent.change(screen.getAllByRole("combobox")[1], {
      target: { value: "integration-jira" },
    });
    expect(screen.getAllByRole("button", { name: "Link MCP" })).toHaveLength(5);
    expect(screen.queryAllByRole("button", { name: "Unlink" })).toHaveLength(0);
    fireEvent.click(screen.getAllByRole("button", { name: "Link MCP" })[0]);

    await waitFor(() => {
      expect(integrationGateway.linkIntegrationToProject).toHaveBeenCalledWith(
        "project-alpha",
        "integration-jira",
      );
      expect(mocks.invalidate).toHaveBeenCalled();
    });
    expect(screen.queryByText(/Edit MCP configuration/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/Add MCP configuration/i)).not.toBeInTheDocument();
  });

  it("navigates to the global MCP page when create new MCP is requested", () => {
    renderSubject();

    fireEvent.click(screen.getByRole("button", { name: "Create new MCP" }));

    expect(mocks.navigate).toHaveBeenCalledWith({ to: "/settings/mcp-servers" });
  });

  it("saves project-level AI provider defaults", async () => {
    const updateProject = vi.fn().mockResolvedValue(buildProject());
    mocks.createGatewayBundle.mockReturnValue({
      integrationGateway: {
        linkIntegrationToProject: vi.fn(),
        unlinkIntegrationFromProject: vi.fn(),
      },
      projectGateway: { updateProject },
      teamGateway: {
        linkTeamToProject: vi.fn(),
        unlinkTeamFromProject: vi.fn(),
      },
    });

    renderSubject();

    fireEvent.click(screen.getByRole("button", { name: "Save defaults" }));

    await waitFor(() => {
      expect(updateProject).toHaveBeenCalledWith("project-alpha", {
        defaultProvider: "codex",
        defaultModel: "gpt-5.4",
        defaultReasoningEffort: "medium",
      });
      expect(mocks.invalidate).toHaveBeenCalled();
    });
  });
});
