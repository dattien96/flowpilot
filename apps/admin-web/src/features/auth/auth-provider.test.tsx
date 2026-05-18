import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

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
    clearAuthSession();
    setAuthLoading(true);
    getSession.mockReset();
    onAuthStateChange.mockReset();
    signOut.mockReset();
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
});
