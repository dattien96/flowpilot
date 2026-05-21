import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ArtifactsPage } from "./artifacts";

const mocks = vi.hoisted(() => ({
  createGatewayBundle: vi.fn(),
  pathname: "/artifacts",
}));

vi.mock("@/data/repository/browser-factory", () => ({
  createGatewayBundle: mocks.createGatewayBundle,
}));

vi.mock("@/components/common/page-frame", () => ({
  PageFrame: ({
    actions,
    children,
    description,
    title,
  }: {
    actions?: ReactNode;
    children?: ReactNode;
    description: string;
    title: string;
  }) => (
    <div>
      <h1>{title}</h1>
      <p>{description}</p>
      {actions}
      {children}
    </div>
  ),
}));

vi.mock("@tanstack/react-router", async () => {
  const actual = await vi.importActual<typeof import("@tanstack/react-router")>(
    "@tanstack/react-router"
  );
  return {
    ...actual,
    createFileRoute: () => () => ({}),
    useLocation: () => ({ pathname: mocks.pathname }),
    Link: ({ children }: { children: ReactNode }) => <>{children}</>,
    Outlet: () => <div>child route</div>,
  };
});

describe("ArtifactsPage", () => {
  beforeEach(() => {
    mocks.createGatewayBundle.mockReset();
    mocks.pathname = "/artifacts";
    mocks.createGatewayBundle.mockReturnValue({
      workflowEngineGateway: {
        listArtifactDefinitions: vi.fn().mockImplementation(() => new Promise(() => undefined)),
        saveArtifactDefinition: vi.fn(),
      },
      localRunnerGateway: {
        getStorageDriver: vi.fn().mockImplementation(() => new Promise(() => undefined)),
      },
    });
  });

  it("renders the child route for nested artifact paths", () => {
    mocks.pathname = "/artifacts/create";

    render(<ArtifactsPage />);

    expect(screen.getByText("child route")).toBeInTheDocument();
    expect(screen.queryByText("Definition table")).not.toBeInTheDocument();
  });
});
