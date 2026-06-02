import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { type ReactNode, useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { AiProvidersContent } from "./ai-providers";
import type { LocalRunnerHealth, LocalRunnerProvider } from "@/domain/model/entity/local-runner";

// ── Router mocks ─────────────────────────────────────────────────────────────

const mocks = vi.hoisted(() => ({
  invalidate: vi.fn(),
  listProviders: vi.fn<[], Promise<LocalRunnerProvider[]>>(),
}));

vi.mock("@tanstack/react-router", async () => {
  const actual = await vi.importActual<typeof import("@tanstack/react-router")>(
    "@tanstack/react-router",
  );
  return {
    ...actual,
    useRouter: () => ({
      invalidate: mocks.invalidate,
    }),
    useRouterState: vi.fn().mockReturnValue(false),
  };
});

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

// Mock createGatewayBundle so refreshInventory mutation hits the mock gateway
vi.mock("@/data/repository/browser-factory", () => ({
  createGatewayBundle: vi.fn(() => ({
    localRunnerGateway: {
      listProviders: mocks.listProviders,
    },
    workflowEngineGateway: {
      listSupportedModels: vi.fn().mockResolvedValue([]),
    },
  })),
}));

// ── Builders ──────────────────────────────────────────────────────────────────

function buildHealth(): LocalRunnerHealth {
  return {
    status: "online",
    runnerVersion: "0.1.0",
    cwd: "C:/working/flowpilot",
    os: "windows",
    startedAt: "2026-05-25T00:00:00.000Z",
    baseUrl: "http://127.0.0.1:4317",
    errorMessage: null,
  };
}

function buildProvider(overrides: Partial<LocalRunnerProvider> = {}): LocalRunnerProvider {
  return {
    key: "codex",
    label: "Codex",
    installed: true,
    version: "1.2.3",
    binaryPath: "C:/tools/codex.cmd",
    authStatus: "READY",
    installHint: "Install the Codex CLI, log in, and restart the runner.",
    supported: true,
    installStatus: "INSTALLED",
    detectedBinary: "codex",
    detectedVersion: "1.2.3",
    models: [
      { id: "gpt-5.5", displayName: "gpt-5.5", available: true, source: "registry" },
      { id: "gpt-5.4", displayName: "gpt-5.4", available: false, source: "registry" },
    ],
    lastError: null,
    ...overrides,
  };
}

// ── Test harness ──────────────────────────────────────────────────────────────

function renderSubject(props: Partial<Parameters<typeof AiProvidersContent>[0]> = {}) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <AiProvidersContent
        health={buildHealth()}
        providers={[buildProvider()]}
        supportedModels={[
          {
            id: "model-1",
            providerKey: "codex",
            modelId: "gpt-5.5",
            displayName: "gpt-5.5",
            isEnabled: true,
            sortOrder: 1,
            source: "registry",
            createdAt: "2026-05-30T00:00:00Z",
          },
          {
            id: "model-2",
            providerKey: "codex",
            modelId: "gpt-5.4",
            displayName: "gpt-5.4",
            isEnabled: false,
            sortOrder: 2,
            source: "registry",
            createdAt: "2026-05-30T00:00:00Z",
          },
        ]}
        {...props}
      />
    </QueryClientProvider>,
  );
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("AI Providers settings", () => {
  beforeEach(() => {
    mocks.invalidate.mockReset();
    mocks.listProviders.mockReset();
    // Default: list returns the same provider
    mocks.listProviders.mockResolvedValue([buildProvider()]);
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({
          providers: [buildProvider()],
        }),
      } as Response),
    );
  });

  it("shows provider inventory details", () => {
    renderSubject();

    expect(screen.getByText("AI Providers")).toBeInTheDocument();
    expect(screen.getByText("Codex")).toBeInTheDocument();
    expect(screen.getByText("INSTALLED")).toBeInTheDocument();
    expect(screen.getByText("READY")).toBeInTheDocument();
    expect(screen.getAllByText("gpt-5.5")[0]).toBeInTheDocument();
    expect(screen.getAllByText("gpt-5.4")[0]).toBeInTheDocument();
  });

  it("refresh button re-detects provider state via GET /providers without calling install", async () => {
    const notInstalledProvider = buildProvider({
      installed: false,
      installStatus: "NOT_INSTALLED",
      authStatus: "UNKNOWN",
      detectedBinary: null,
      detectedVersion: null,
      lastError: "gemini removed from PATH",
    });

    // Simulate runner returning NOT_INSTALLED after manual uninstall
    mocks.listProviders.mockResolvedValueOnce([notInstalledProvider]);

    renderSubject({ providers: [buildProvider()] });

    // Click the per-provider "Refresh" button (INSTALLED → detect-only)
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));

    await waitFor(() => {
      // Must NOT have called the install endpoint
      expect(global.fetch).not.toHaveBeenCalledWith(
        "/api/local-runner/providers/install",
        expect.anything(),
      );
      // Must have called listProviders on the gateway
      expect(mocks.listProviders).toHaveBeenCalled();
    });

    await waitFor(() => {
      expect(screen.getByText("NOT_INSTALLED")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Install" })).toBeInTheDocument();
      expect(screen.getByText("gemini removed from PATH")).toBeInTheDocument();
    });
  });

  it("install button calls the install API for NOT_INSTALLED providers", async () => {
    const notInstalledProvider = buildProvider({
      installed: false,
      installStatus: "NOT_INSTALLED",
      authStatus: "UNKNOWN",
      detectedBinary: null,
      detectedVersion: null,
      lastError: null,
    });
    const installedAfter = buildProvider();

    vi.mocked(global.fetch).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ providers: [installedAfter] }),
    } as Response);

    renderSubject({ providers: [notInstalledProvider] });

    fireEvent.click(screen.getByRole("button", { name: "Install" }));

    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith("/api/local-runner/providers/install", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ providerName: "codex" }),
      });
    });

    await waitFor(() => {
      expect(screen.getByText("INSTALLED")).toBeInTheDocument();
    });
  });

  it("auth button is shown when auth is required, and triggers the auth API", async () => {
    const authRequiredProvider = buildProvider({
      installed: true,
      installStatus: "INSTALLED",
      authStatus: "AUTH_REQUIRED",
    });

    vi.mocked(global.fetch).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ status: "success" }),
    } as Response);

    renderSubject({ providers: [authRequiredProvider] });

    // Both Auth and Refresh buttons must be present
    expect(screen.getByRole("button", { name: "Auth" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Refresh" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Auth" }));

    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith("/api/local-runner/providers/auth", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ providerName: "codex" }),
      });
    });
  });
});
