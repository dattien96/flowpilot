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
    localStorage.clear();
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

    fireEvent.click(await screen.findByText("owner@example.com"));
    fireEvent.click(await screen.findByRole("button", { name: "Disconnect account" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/runtime/google-drive-config/accounts/account-1",
        expect.objectContaining({ method: "DELETE" }),
      );
    });
    expect(await screen.findByText("No Google accounts are connected on this runner yet. Start a new account connection, finish OAuth in the popup, then come back here to select the proxy MCP account or bind project artifact folders.")).toBeInTheDocument();
  });

  it("collapses account items by default and resets expanded state when step changes", async () => {
    mocks.loadGoogleDriveRuntimeStatus.mockResolvedValue(createRuntimeStatus());
    renderPage();

    // Initially, account is collapsed, so "Disconnect account" is not in the document.
    expect(screen.queryByRole("button", { name: "Disconnect account" })).not.toBeInTheDocument();

    // Click the header to expand
    fireEvent.click(await screen.findByText("owner@example.com"));
    expect(await screen.findByRole("button", { name: "Disconnect account" })).toBeInTheDocument();

    // Switch to Step 1
    fireEvent.click(screen.getByText("Step 1"));
    // Verify Step 1 content is visible
    expect(await screen.findByText("Use one Google Cloud project for this MVP and keep the active project selected.")).toBeInTheDocument();

    // Switch back to Step 6
    fireEvent.click(screen.getByText("Step 6"));
    
    // Account should be collapsed again
    expect(await screen.findByText("owner@example.com")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Disconnect account" })).not.toBeInTheDocument();
  });

  it("handles manual confirmation on Steps 1, 2, and 3", async () => {
    // Start with all unconfigured/empty
    const status = createRuntimeStatus({
      artifactSync: {
        status: "needs_input",
        source: "local",
        configured: false,
        clientId: "",
        redirectUri: "",
        hasClientSecret: false,
        hasPickerApiKey: false,
        missingFields: [],
      },
    });
    mocks.loadGoogleDriveRuntimeStatus.mockResolvedValue(status);
    
    const localStorageSpySet = vi.spyOn(Storage.prototype, "setItem");

    renderPage();

    // Verify step 1 instructions are visible
    expect(await screen.findByText("Use one Google Cloud project for this MVP and keep the active project selected.")).toBeInTheDocument();

    // Click "Confirm it done" on step 1
    fireEvent.click(screen.getByRole("button", { name: "Confirm it done" }));
    expect(localStorageSpySet).toHaveBeenCalledWith("flowpilot_gdrive_step1_done", "true");

    // Verify active step is now step 2
    expect(await screen.findByText("Use External for personal testing, add test users, and switch to Production when you are done testing.")).toBeInTheDocument();

    // Click "Confirm it done" on step 2
    fireEvent.click(screen.getByRole("button", { name: "Confirm it done" }));
    expect(localStorageSpySet).toHaveBeenCalledWith("flowpilot_gdrive_step2_done", "true");

    // Verify active step is now step 3
    expect(await screen.findByText("Enable Google Drive API, Google Picker API, and the Docs/Sheets/Slides APIs if the MCP tools need them.")).toBeInTheDocument();

    // Click "Confirm it done" on step 3
    fireEvent.click(screen.getByRole("button", { name: "Confirm it done" }));
    expect(localStorageSpySet).toHaveBeenCalledWith("flowpilot_gdrive_step3_done", "true");

    // Verify active step is now step 4
    expect(await screen.findByText("Save the Web OAuth client locally so FlowPilot can reuse it for artifact sync and the proxy MCP without editing .env.")).toBeInTheDocument();
  });

  it("supports navigation via Next Step buttons on Steps 1 to 6", async () => {
    const status = createRuntimeStatus({
      artifactSync: {
        status: "needs_input",
        source: "local",
        configured: false,
        clientId: "",
        redirectUri: "",
        hasClientSecret: false,
        hasPickerApiKey: false,
        missingFields: [],
      },
      mcp: {
        status: "needs_input",
        configured: false,
        proxyMcpEnabled: true,
        credentialPath: "/tmp/gcp-oauth.keys.json",
        tokenPath: "/tmp/tokens.json",
        credentialFileExists: false,
        credentialFileValid: false,
        tokenFileExists: false,
        tokenRefreshValid: false,
        needsAuth: true,
        backendPackageAvailable: true,
        accountId: "",
        accountEmail: "",
        accountSelectionRequired: true,
        grantedScopes: [],
        missingScopes: [],
        accountReady: false,
        artifactBindingPresent: false,
        artifactReady: false,
        mcpReadReady: false,
        mcpWriteReady: false,
        reconnectRequired: false,
        missingFields: [],
      },
      accounts: [],
    });
    mocks.loadGoogleDriveRuntimeStatus.mockResolvedValue(status);
    renderPage();
    // Wait for initial load to finish so setActiveStep doesn't get reset
    expect(await screen.findByText("Use one Google Cloud project for this MVP and keep the active project selected.")).toBeInTheDocument();

    // Step 1: Click "Next Step"
    fireEvent.click(await screen.findByRole("button", { name: "Next Step" }));
    expect(await screen.findByText("Use External for personal testing, add test users, and switch to Production when you are done testing.")).toBeInTheDocument();

    // Step 2: Click "Next Step"
    fireEvent.click(screen.getByRole("button", { name: "Next Step" }));
    expect(await screen.findByText("Enable Google Drive API, Google Picker API, and the Docs/Sheets/Slides APIs if the MCP tools need them.")).toBeInTheDocument();

    // Step 3: Click "Next Step"
    fireEvent.click(screen.getByRole("button", { name: "Next Step" }));
    expect(await screen.findByText("Save the Web OAuth client locally so FlowPilot can reuse it for artifact sync and the proxy MCP without editing .env.")).toBeInTheDocument();

    // Step 4: Click "Next Step"
    fireEvent.click(screen.getByRole("button", { name: "Next Step" }));
    expect(await screen.findByText("Save the browser-side Picker API key separately from the OAuth client secret.")).toBeInTheDocument();

    // Step 5: Click "Next Step"
    fireEvent.click(screen.getByRole("button", { name: "Next Step" }));
    expect(await screen.findByText("Select Google account for proxy MCP")).toBeInTheDocument();

    // Step 6: Click "Next Step"
    fireEvent.click(screen.getByRole("button", { name: "Next Step" }));
    expect(await screen.findByText("Google Drive proxy MCP setup")).toBeInTheDocument();
  });
});
