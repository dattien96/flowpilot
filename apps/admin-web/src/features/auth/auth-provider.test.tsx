import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AuthProvider, useAuth } from "./auth-provider";
import { clearAuthSession, setAuthLoading } from "./use-auth";

const { getSession, onAuthStateChange, signOut } = vi.hoisted(() => ({
  getSession: vi.fn(),
  onAuthStateChange: vi.fn(),
  signOut: vi.fn(),
}));

vi.mock("@/data/supabase/client", () => ({
  supabase: {
    auth: {
      getSession,
      onAuthStateChange,
      signOut,
    },
  },
}));

vi.mock("@/lib/env/browser-env", () => ({
  hasSupabaseEnv: vi.fn(),
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
    getSession.mockReset();
    onAuthStateChange.mockReset();
    signOut.mockReset();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("hydrates auth state from the current Supabase session on mount", async () => {
    const { hasSupabaseEnv } = await import("@/lib/env/browser-env");
    vi.mocked(hasSupabaseEnv).mockReturnValue(true);
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

  it("uses demo session when Supabase env is unavailable", async () => {
    const { hasSupabaseEnv } = await import("@/lib/env/browser-env");
    vi.mocked(hasSupabaseEnv).mockReturnValue(false);

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
    const { hasSupabaseEnv } = await import("@/lib/env/browser-env");
    vi.mocked(hasSupabaseEnv).mockReturnValue(true);
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
    const { hasSupabaseEnv } = await import("@/lib/env/browser-env");
    vi.mocked(hasSupabaseEnv).mockReturnValue(true);
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
