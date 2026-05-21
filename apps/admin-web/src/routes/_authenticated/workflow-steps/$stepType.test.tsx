import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { WorkflowStepDetailPage } from "./$stepType";
import type { StepDefinition } from "@/domain/model/entity/workflow-engine";

const mocks = vi.hoisted(() => ({
  createGatewayBundle: vi.fn(),
  params: { stepType: "tech_spec" } as { stepType: string },
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
    createFileRoute: () => () => ({
      useParams: () => mocks.params,
    }),
    Link: ({ children }: { children: ReactNode }) => <>{children}</>,
  };
});

function buildStep(overrides: Partial<StepDefinition> = {}): StepDefinition {
  return {
    stepType: "tech_spec",
    name: "Tech Spec",
    description: "Produce technical layout",
    requiredMcps: ["jira"],
    requiredSkills: ["tech_spec_skill"],
    agentType: "standard",
    createdAt: "2026-05-20T00:00:00.000Z",
    updatedAt: "2026-05-21T00:00:00.000Z",
    ...overrides,
  };
}

describe("WorkflowStepDetailPage", () => {
  beforeEach(() => {
    mocks.createGatewayBundle.mockReset();
    mocks.params = { stepType: "tech_spec" };
    vi.spyOn(window, "alert").mockImplementation(() => undefined);
  });

  it("loads and saves an existing step definition", async () => {
    const step = buildStep();
    const gatewayBundle = {
      workflowEngineGateway: {
        listStepDefinitions: vi.fn().mockResolvedValue([step]),
        saveStepDefinition: vi.fn().mockResolvedValue(
          buildStep({
            name: "Updated Tech Spec",
            description: "Updated description",
          })
        ),
      },
    };
    mocks.createGatewayBundle.mockReturnValue(gatewayBundle);

    render(<WorkflowStepDetailPage />);

    expect(await screen.findByDisplayValue("Tech Spec")).toBeInTheDocument();
    expect(screen.getByDisplayValue("tech_spec")).toBeDisabled();
    expect(screen.getByText("Jira")).toBeInTheDocument();

    fireEvent.change(screen.getByDisplayValue("Tech Spec"), {
      target: { value: "Updated Tech Spec" },
    });
    fireEvent.change(screen.getByDisplayValue("Produce technical layout"), {
      target: { value: "Updated description" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Remove" }));
    fireEvent.change(screen.getByRole("combobox", { name: /available mcp/i }), {
      target: { value: "figma" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add MCP" }));
    fireEvent.click(screen.getByRole("button", { name: "Save step" }));

    await waitFor(() => {
      expect(gatewayBundle.workflowEngineGateway.saveStepDefinition).toHaveBeenCalledWith(
        expect.objectContaining({
          stepType: "tech_spec",
          name: "Updated Tech Spec",
          description: "Updated description",
          requiredMcps: ["figma"],
        })
      );
    });
  });
});
