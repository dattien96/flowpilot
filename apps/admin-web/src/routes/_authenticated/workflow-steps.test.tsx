import { fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { WorkflowStepsPage } from "./workflow-steps";
import type { StepDefinition } from "@/domain/model/entity/workflow-engine";

const mocks = vi.hoisted(() => ({
  createGatewayBundle: vi.fn(),
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
    children: ReactNode;
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
    useLocation: () => ({ pathname: "/workflow-steps" }),
    Link: ({ children }: { children: ReactNode }) => <>{children}</>,
    Outlet: () => null,
  };
});

function buildStep(overrides: Partial<StepDefinition> = {}): StepDefinition {
  return {
    stepType: "tech_spec",
    name: "Tech Spec",
    description: "Produce technical layout",
    requiredMcps: [],
    requiredSkills: [],
    inputArtifactDefinitions: [],
    outputArtifactDefinitions: ["tech_spec_artifact"],
    agentType: "standard",
    createdAt: "2026-05-20T00:00:00.000Z",
    updatedAt: "2026-05-20T00:00:00.000Z",
    ...overrides,
  };
}

describe("WorkflowStepsPage", () => {
  beforeEach(() => {
    mocks.createGatewayBundle.mockReset();
  });

  it("sorts step definitions by the selected order", async () => {
    mocks.createGatewayBundle.mockReturnValue({
      workflowEngineGateway: {
        listStepDefinitions: vi.fn().mockResolvedValue([
          buildStep({
            stepType: "zeta",
            name: "Zeta Step",
            outputArtifactDefinitions: ["coding_plan_artifact"],
            createdAt: "2026-05-18T00:00:00.000Z",
            updatedAt: "2026-05-18T00:00:00.000Z",
          }),
          buildStep({
            stepType: "alpha",
            name: "Alpha Step",
            inputArtifactDefinitions: ["business_summary_artifact"],
            createdAt: "2026-05-20T00:00:00.000Z",
            updatedAt: "2026-05-20T00:00:00.000Z",
          }),
        ]),
        listArtifactDefinitions: vi.fn().mockResolvedValue([
          {
            key: "business_summary_artifact",
            name: "Business Summary",
            description: "",
            localPathTemplate: "",
            remotePathTemplate: "",
            defaultFileName: "BusinessSummary.md",
            createdAt: "2026-05-20T00:00:00Z",
            updatedAt: "2026-05-20T00:00:00Z",
          },
          {
            key: "coding_plan_artifact",
            name: "Coding Plan",
            description: "",
            localPathTemplate: "",
            remotePathTemplate: "",
            defaultFileName: "CodingPlan.md",
            createdAt: "2026-05-20T00:00:00Z",
            updatedAt: "2026-05-20T00:00:00Z",
          },
          {
            key: "tech_spec_artifact",
            name: "Tech Spec",
            description: "",
            localPathTemplate: "",
            remotePathTemplate: "",
            defaultFileName: "TechSpec.md",
            createdAt: "2026-05-20T00:00:00Z",
            updatedAt: "2026-05-20T00:00:00Z",
          },
        ]),
      },
    });

    render(<WorkflowStepsPage />);

    expect(await screen.findByText("Zeta Step")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Idea - Business - Architecture - Tech Spec - Code Plan - TDD - Coding Implementation Checklist - Review"
      )
    ).toBeInTheDocument();

    let names = screen
      .getAllByRole("heading", { level: 3 })
      .map((heading) => heading.textContent);
    expect(names).toEqual(["Alpha Step", "Zeta Step"]);

    fireEvent.change(screen.getByRole("combobox", { name: /sort by/i }), {
      target: { value: "updated-asc" },
    });

    names = screen
      .getAllByRole("heading", { level: 3 })
      .map((heading) => heading.textContent);
    expect(names).toEqual(["Zeta Step", "Alpha Step"]);

    expect(screen.getByText("Input artifacts: Business Summary")).toBeInTheDocument();
    expect(screen.getByText("Output artifacts: Coding Plan")).toBeInTheDocument();
  });
});
