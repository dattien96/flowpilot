import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { McpServersOverviewPage } from "./mcp-servers";

const mocks = vi.hoisted(() => ({
  pathname: "/settings/mcp-servers",
  useLoaderData: vi.fn(),
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
    useLocation: () => ({ pathname: mocks.pathname }),
    Outlet: () => <div>Outlet Content</div>,
    createFileRoute: () => () => ({
      useLoaderData: mocks.useLoaderData,
    }),
    Link: ({ children, to, ...props }: any) => (
      <a href={to} {...props}>
        {children}
      </a>
    ),
  };
});

describe("MCP Servers overview route", () => {
  it("renders the overview cards on the base route", () => {
    mocks.pathname = "/settings/mcp-servers";
    mocks.useLoaderData.mockReturnValue({
      allIntegrations: [{ id: "1", status: "connected" }, { id: "2", status: "failed" }],
      backends: [{ key: "jira" }, { key: "google_drive" }],
      health: { status: "online", baseUrl: "http://127.0.0.1:4317" },
    });

    render(<McpServersOverviewPage />);

    expect(screen.getByText("MCP settings now live in child pages")).toBeInTheDocument();
    expect(screen.getByText("Google Console - Driver")).toBeInTheDocument();
    expect(screen.getByText("Jira")).toBeInTheDocument();
    expect(screen.getByText("Connected / Total")).toBeInTheDocument();
  });

  it("renders the outlet on child routes", () => {
    mocks.pathname = "/settings/mcp-servers/jira-link";
    mocks.useLoaderData.mockReturnValue({
      allIntegrations: [],
      backends: [],
      health: { status: "online", baseUrl: "http://127.0.0.1:4317" },
    });

    render(<McpServersOverviewPage />);

    expect(screen.getByText("Outlet Content")).toBeInTheDocument();
    expect(screen.queryByText("MCP settings now live in child pages")).not.toBeInTheDocument();
  });
});
