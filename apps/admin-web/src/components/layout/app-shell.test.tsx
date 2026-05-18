import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { AppShell } from "./app-shell";

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    children,
    className,
    to,
  }: {
    children: React.ReactNode;
    className?: string;
    to: string;
  }) => (
    <a className={className} href={to}>
      {children}
    </a>
  ),
  useLocation: () => ({
    pathname: "/dashboard",
  }),
}));

vi.mock("@/features/auth/auth-provider", () => ({
  useAuth: () => ({
    loading: false,
    session: {
      mode: "demo",
      user: {
        email: "demo@flowpilot.local",
        id: "demo-user",
      },
    },
    signOut: vi.fn(),
  }),
}));

describe("AppShell", () => {
  it("renders sidebar, header, and child content regions", () => {
    render(
      <AppShell>
        <div>Child content</div>
      </AppShell>,
    );

    expect(screen.getByText("Foundation Shell")).toBeInTheDocument();
    expect(screen.getByText("Dashboard")).toBeInTheDocument();
    expect(screen.getByText("Projects")).toBeInTheDocument();
    expect(screen.getByText("Integrations")).toBeInTheDocument();
    expect(screen.getByText("Child content")).toBeInTheDocument();
  });
});
