// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { GoogleDriveSetupPage } from "./google-drive-setup";
import type { GoogleDriveRuntimeStatus } from "@/lib/google-drive/runtime-config";

const mocks = vi.hoisted(() => ({
  loaderData: {
    allIntegrations: [],
    backends: [],
    health: {
      status: "online",
      baseUrl: "http://127.0.0.1:4317",
    },
    projects: [
      {
        id: "project-1",
        name: "Project One",
      },
    ],
  },
  invalidate: vi.fn(),
  navigate: vi.fn(),
  loadGoogleDriveRuntimeStatus: vi.fn(),
  createGatewayBundle: vi.fn(),
}));

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children?: ReactNode }) => <>{children}</>,
  createFileRoute: () => () => ({
    useLoaderData: () => mocks.loaderData,
  }),
  useRouter: () => ({
    invalidate: mocks.invalidate,
    navigate: mocks.navigate,
  }),
}));

vi.mock("@/features/mcp/mcp-settings-loader", () => ({
  loadMcpSettingsData: vi.fn(),
}));

vi.mock("@/lib/google-drive/runtime-config", () => ({
  loadGoogleDriveRuntimeStatus: mocks.loadGoogleDriveRuntimeStatus,
}));

vi.mock("@/data/repository/browser-factory", () => ({
  createGatewayBundle: mocks.createGatewayBundle,
}));

vi.mock("@/components/common/page-frame", () => ({
  PageFrame: ({
    title,
    description,
    children,
  }: {
    title: string;
    description?: string;
    children?: ReactNode;
  }) => (
    <div>
      <h1>{title}</h1>
      {description ? <p>{description}</p> : null}
      {children}
    </div>
  ),
}));

vi.mock("@/presentation/components/ui/badge", () => ({
  Badge: ({ children }: { children?: ReactNode }) => <span>{children}</span>,
}));

vi.mock("./components/-GoogleDriveProviderConfigCard", () => ({
  GoogleDriveProviderConfigCard: () => <div>provider config card</div>,
}));

function createRuntimeStatus(overrides?: Partial<GoogleDriveRuntimeStatus>): GoogleDriveRuntimeStatus {
  return {
    artifactSync: {
      status: "configured",
      source: "local",
      configured: true,
      clientId: "client-id",
      redirectUri: "http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback",
      hasClientSecret: true,
      hasPickerApiKey: true,
      missingFields: [],
    },
    mcp: {
      status: "needs_auth",
      configured: false,
      proxyMcpEnabled: true,
      credentialPath: "/tmp/gcp-oauth.keys.json",
      tokenPath: "/tmp/tokens.json",
      credentialFileExists: false,
      credentialFileValid: false,
      tokenFileExists: false,
      tokenRefreshValid: true,
      needsAuth: true,
      backendPackageAvailable: true,
      accountId: "account-1",
      accountEmail: "owner@example.com",
      accountSelectionRequired: false,
      grantedScopes: ["https://www.googleapis.com/auth/drive.file", "openid", "email"],
      missingScopes: ["https://www.googleapis.com/auth/drive.readonly"],
      accountReady: true,
      artifactBindingPresent: false,
      artifactReady: false,
      mcpReadReady: false,
      mcpWriteReady: true,
      reconnectRequired: false,
      missingFields: [],
    },
    accounts: [
      {
        accountId: "account-1",
        accountEmail: "owner@example.com",
        oauthClientId: "client-id",
        status: "connected",
        projectCount: 1,
        accountReady: true,
        mcpReadReady: false,
        mcpWriteReady: true,
        grantedScopes: ["https://www.googleapis.com/auth/drive.file", "openid", "email"],
        missingScopes: ["https://www.googleapis.com/auth/drive.readonly"],
        connectedAt: "2026-06-10T00:00:00.000Z",
        updatedAt: "2026-06-10T01:00:00.000Z",
      },
    ],
    providerConfigs: [],
    runnerReachable: true,
    lastError: null,
    ...overrides,
  };
}

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <GoogleDriveSetupPage />
    </QueryClientProvider>,
  );
}

describe("GoogleDriveSetupPage", () => {
  beforeEach(() => {
    mocks.invalidate.mockReset();
    mocks.navigate.mockReset();
    mocks.createGatewayBundle.mockReset();
    mocks.loadGoogleDriveRuntimeStatus.mockReset();
    mocks.createGatewayBundle.mockResolvedValue({
      localRunnerGateway: {
        installMcpBackend: vi.fn(),
        triggerMcpBackendAction: vi.fn(),
      },
      integrationGateway: {
        updateIntegration: vi.fn(),
      },
    });
    vi.unstubAllGlobals();
  });

  it("starts a new account connect session and shows capability gaps", async () => {
    mocks.loadGoogleDriveRuntimeStatus.mockResolvedValue(createRuntimeStatus());
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({
        sessionId: "session-1",
        status: "pending",
        connectUrl: "http://127.0.0.1:4317/artifact-storage/google-drive/connect?sessionId=session-1",
      }),
    });
    const openMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("open", openMock);

    renderPage();

    expect(await screen.findByText("Select Google account for proxy MCP")).toBeInTheDocument();
    expect(screen.getAllByText(/Missing scopes: https:\/\/www.googleapis.com\/auth\/drive.readonly/).length).toBeGreaterThan(0);

    fireEvent.click(screen.getByRole("button", { name: "Connect Google account" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/runtime/google-drive-config/accounts/connect-session",
        expect.objectContaining({ method: "POST" }),
      );
    });
    expect(openMock).toHaveBeenCalledWith(
      "http://127.0.0.1:4317/artifact-storage/google-drive/connect?sessionId=session-1",
      "_blank",
      "width=980,height=820",
    );
  });

  it("disconnects an existing account from the settings page", async () => {
    mocks.loadGoogleDriveRuntimeStatus
      .mockResolvedValueOnce(createRuntimeStatus())
      .mockResolvedValueOnce(createRuntimeStatus({
        mcp: {
          ...createRuntimeStatus().mcp,
          accountId: "",
          accountEmail: "",
          accountSelectionRequired: true,
          status: "needs_input",
        },
        accounts: [],
      }));

    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 204,
      json: vi.fn().mockResolvedValue({}),
    });
    vi.stubGlobal("fetch", fetchMock);

    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Disconnect account" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/runtime/google-drive-config/accounts/account-1",
        expect.objectContaining({ method: "DELETE" }),
      );
    });
    expect(await screen.findByText("No Google accounts are connected on this runner yet. Start a new account connection, finish OAuth in the popup, then come back here to select the account for proxy MCP and artifact binding.")).toBeInTheDocument();
  });
});
