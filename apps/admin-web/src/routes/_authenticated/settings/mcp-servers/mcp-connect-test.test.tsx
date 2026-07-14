import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { McpConnectTestRetiredPage } from "./mcp-connect-test";

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: { component: unknown }) => opts,
  Link: ({ children, to }: { children: ReactNode; to: string }) => <a href={to}>{children}</a>,
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

describe("McpConnectTestRetiredPage", () => {
  it("shows retired notice and links to Jira MCP settings", () => {
    render(<McpConnectTestRetiredPage />);
    expect(screen.getByText(/MCP Test Console \(retired\)/i)).toBeInTheDocument();
    expect(screen.getByText(/Rovo remote MCP/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Jira MCP settings/i })).toHaveAttribute(
      "href",
      "/settings/mcp-servers/jira-link",
    );
    expect(screen.getByRole("link", { name: /Enable Rovo MCP API token/i })).toBeInTheDocument();
  });
});
