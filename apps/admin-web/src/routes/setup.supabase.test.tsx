import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { SetupSupabasePage } from "./setup.supabase";

const { resetBrowserGatewayBundle } = vi.hoisted(() => ({
  resetBrowserGatewayBundle: vi.fn(),
}));

const { resetBrowserSupabaseClient } = vi.hoisted(() => ({
  resetBrowserSupabaseClient: vi.fn(),
}));

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, ...props }: React.AnchorHTMLAttributes<HTMLAnchorElement>) => (
    <a {...props}>{children}</a>
  ),
  createFileRoute: () => () => ({}),
}));

vi.mock("@/data/repository/browser-factory", () => ({
  resetBrowserGatewayBundle,
}));

vi.mock("@/data/supabase/client", () => ({
  resetBrowserSupabaseClient,
}));

describe("SetupSupabasePage", () => {
  beforeEach(() => {
    resetBrowserGatewayBundle.mockReset();
    resetBrowserSupabaseClient.mockReset();
    vi.restoreAllMocks();
  });

  it("resets browser caches and performs a hard login redirect after save", async () => {
    vi.spyOn(window, "fetch")
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            valid: true,
            checks: [],
          }),
          {
            status: 200,
            headers: { "content-type": "application/json" },
          },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ ok: true }), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      );

    const assign = vi.fn();
    Object.defineProperty(window, "location", {
      configurable: true,
      value: {
        assign,
      },
    });

    render(<SetupSupabasePage />);

    fireEvent.change(screen.getByPlaceholderText("https://project-ref.supabase.co"), {
      target: { value: "https://demo-ref.supabase.co" },
    });
    fireEvent.change(screen.getByPlaceholderText("Public anon key"), {
      target: { value: "anon-key" },
    });
    fireEvent.change(screen.getByPlaceholderText("https://project-ref.supabase.co/functions/v1"), {
      target: { value: "https://demo-ref.supabase.co/functions/v1" },
    });
    fireEvent.change(screen.getByPlaceholderText("Privileged service role key"), {
      target: { value: "service-role-key" },
    });

    fireEvent.click(screen.getByText("Test Connection"));

    await waitFor(() => {
      expect(screen.getByText("Connection checks passed.")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Save & Configure"));

    await waitFor(() => {
      expect(resetBrowserGatewayBundle).toHaveBeenCalledTimes(1);
      expect(resetBrowserSupabaseClient).toHaveBeenCalledTimes(1);
      expect(assign).toHaveBeenCalledWith("/login");
    });
  });
});
