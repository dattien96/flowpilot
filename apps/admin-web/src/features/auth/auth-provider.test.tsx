import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AuthProvider, useAuth } from "./auth-provider";
import { clearAuthSession, setAuthLoading } from "./use-auth";

const { getBrowserSupabaseClient, getSession, loadSupabaseRuntimeStatus, onAuthStateChange, signOut } = vi.hoisted(() => ({
  getBrowserSupabaseClient: vi.fn(),
  getSession: vi.fn(),
  loadSupabaseRuntimeStatus: vi.fn(),
  onAuthStateChange: vi.fn(),
  signOut: vi.fn(),
}));

vi.mock("@/data/supabase/client", () => ({
  getBrowserSupabaseClient,
}));

vi.mock("@/lib/supabase/runtime-config", () => ({
  loadSupabaseRuntimeStatus,
}));

function Probe() {
  const { loading, session } = useAuth();

  return (
    <div>
      <span data-testid="loading">{String(loading)}</span>
      <span data-testid="mode">{session?.mode ?? "none"}</span>
      <span data-testid="email">{session?.user.email ?? "none"}</span>
    </div>
  );
}

describe("AuthProvider", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.spyOn(console, "error").mockImplementation(() => {});
    clearAuthSession();
    setAuthLoading(true);
    getBrowserSupabaseClient.mockReset();
    getSession.mockReset();
    loadSupabaseRuntimeStatus.mockReset();
    onAuthStateChange.mockReset();
    signOut.mockReset();
    getBrowserSupabaseClient.mockResolvedValue({
      auth: {
        getSession,
        onAuthStateChange,
        signOut,
      },
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("hydrates auth state from the current Supabase session on mount", async () => {
    loadSupabaseRuntimeStatus.mockResolvedValue(supabaseRuntimeStatus());
    getSession.mockResolvedValue({
      data: {
        session: {
          user: {
            email: "admin@flowpilot.local",
            id: "user-1",
          },
        },
      },
    });
    onAuthStateChange.mockReturnValue({
      data: {
        subscription: {
          unsubscribe: vi.fn(),
        },
      },
    });

    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("loading")).toHaveTextContent("false");
      expect(screen.getByTestId("mode")).toHaveTextContent("supabase");
      expect(screen.getByTestId("email")).toHaveTextContent("admin@flowpilot.local");
    });
  });

  it("uses demo session when runtime config is unavailable", async () => {
    loadSupabaseRuntimeStatus.mockResolvedValue(demoRuntimeStatus());

    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("loading")).toHaveTextContent("false");
      expect(screen.getByTestId("mode")).toHaveTextContent("demo");
      expect(screen.getByTestId("email")).toHaveTextContent("demo@flowpilot.local");
    });
  });

  it("clears the loading state when reading the session fails", async () => {
    loadSupabaseRuntimeStatus.mockResolvedValue(supabaseRuntimeStatus());
    getSession.mockRejectedValue(new Error("network down"));
    onAuthStateChange.mockReturnValue({
      data: {
        subscription: {
          unsubscribe: vi.fn(),
        },
      },
    });

    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("loading")).toHaveTextContent("false");
      expect(screen.getByTestId("mode")).toHaveTextContent("none");
      expect(screen.getByTestId("email")).toHaveTextContent("none");
    });
  });

  it("hydrates auth state from the auth event payload without re-reading the session", async () => {
    loadSupabaseRuntimeStatus.mockResolvedValue(supabaseRuntimeStatus());
    getSession.mockResolvedValue({
      data: {
        session: null,
      },
    });

    let authStateCallback:
      | ((event: string, session: { user: { email: string; id: string } } | null) => void)
      | undefined;

    onAuthStateChange.mockImplementation((callback) => {
      authStateCallback = callback;
      return {
        data: {
          subscription: {
            unsubscribe: vi.fn(),
          },
        },
      };
    });

    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("loading")).toHaveTextContent("false");
      expect(screen.getByTestId("mode")).toHaveTextContent("none");
    });

    expect(getSession).toHaveBeenCalledTimes(1);

    authStateCallback?.("SIGNED_IN", {
      user: {
        email: "admin@flowpilot.local",
        id: "user-1",
      },
    });

    await waitFor(() => {
      expect(screen.getByTestId("mode")).toHaveTextContent("supabase");
      expect(screen.getByTestId("email")).toHaveTextContent("admin@flowpilot.local");
    });

    expect(getSession).toHaveBeenCalledTimes(1);
  });
});

function supabaseRuntimeStatus() {
  return {
    mode: "config",
    configured: true,
    apiUrl: "https://demo-ref.supabase.co",
    anonKey: "anon-key",
    edgeFunctionUrl: "https://demo-ref.supabase.co/functions/v1",
    hasServiceRoleKey: true,
    projectRef: "demo-ref",
    runnerReachable: true,
    envAvailable: false,
    savedConfigAvailable: true,
    edgeFunctionsReady: true,
    lastError: null,
  };
}

function demoRuntimeStatus() {
  return {
    ...supabaseRuntimeStatus(),
    mode: "demo",
    configured: false,
    apiUrl: null,
    anonKey: null,
    edgeFunctionUrl: null,
    hasServiceRoleKey: false,
    projectRef: null,
    savedConfigAvailable: false,
    edgeFunctionsReady: false,
  };
}
