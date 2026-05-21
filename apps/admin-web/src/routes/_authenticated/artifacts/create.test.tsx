import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { CreateArtifactPage } from "./create";

const mocks = vi.hoisted(() => ({
  createGatewayBundle: vi.fn(),
  navigate: vi.fn(),
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
    Link: ({ children }: { children: ReactNode }) => <>{children}</>,
    useNavigate: () => mocks.navigate,
  };
});

describe("CreateArtifactPage", () => {
  beforeEach(() => {
    mocks.createGatewayBundle.mockReset();
    mocks.navigate.mockReset();
    vi.spyOn(window, "alert").mockImplementation(() => undefined);
  });

  it("creates a new artifact definition and returns to the artifact list", async () => {
    const gatewayBundle = {
      workflowEngineGateway: {
        saveArtifactDefinition: vi.fn().mockImplementation(async (definition) => definition),
      },
    };
    mocks.createGatewayBundle.mockReturnValue(gatewayBundle);
    mocks.navigate.mockResolvedValue(undefined);

    render(<CreateArtifactPage />);

    fireEvent.change(screen.getByLabelText("Key"), {
      target: { value: "plan_artifact" },
    });
    fireEvent.change(screen.getByLabelText("Name"), {
      target: { value: "Plan" },
    });
    fireEvent.change(screen.getByLabelText("Description"), {
      target: { value: "Plan artifact" },
    });
    fireEvent.change(screen.getByLabelText("Local path template"), {
      target: { value: ".flowpilot/artifacts/{projectId}/Plan.md" },
    });
    fireEvent.change(screen.getByLabelText("Remote path template"), {
      target: { value: "artifacts/{projectId}/Plan.md" },
    });
    fireEvent.change(screen.getByLabelText("Default file name"), {
      target: { value: "Plan.md" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create Artifact" }));

    await waitFor(() => {
      expect(gatewayBundle.workflowEngineGateway.saveArtifactDefinition).toHaveBeenCalledWith(
        expect.objectContaining({
          key: "plan_artifact",
          name: "Plan",
          defaultFileName: "Plan.md",
        })
      );
      expect(mocks.navigate).toHaveBeenCalledWith({ to: "/artifacts" });
    });
  });
});
