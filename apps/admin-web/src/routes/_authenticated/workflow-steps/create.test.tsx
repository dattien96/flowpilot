import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { CreateWorkflowStepPage } from "./create";
import type { StepDefinition } from "@/domain/model/entity/workflow-engine";

const mocks = vi.hoisted(() => ({
  createGatewayBundle: vi.fn(),
  navigate: vi.fn().mockResolvedValue(undefined),
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

vi.mock("@/features/workflow-engine/artifact-definition-selector", () => ({
  ArtifactDefinitionSelector: ({
    label,
    artifactDefinitions,
    selectedArtifactKeys,
    onChange,
  }: {
    label: string;
    artifactDefinitions: { key: string; label: string }[];
    selectedArtifactKeys: string[];
    onChange: (next: string[]) => void;
  }) => {
    const nextKey = label.startsWith("Input")
      ? "business_summary_artifact"
      : "tech_spec_artifact";

    return (
      <div>
        <h2>{label}</h2>
        <p>{artifactDefinitions.map((definition) => definition.key).join(",")}</p>
        <p>{selectedArtifactKeys.join(",")}</p>
        <button type="button" onClick={() => onChange([...selectedArtifactKeys, nextKey])}>
          Add artifact
        </button>
      </div>
    );
  },
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

function buildStep(overrides: Partial<StepDefinition> = {}): StepDefinition {
  return {
    stepType: "tech_spec",
    name: "Tech Spec",
    description: "Produce technical layout",
    promptBase: "Produce technical layout for the workflow.",
    requiredMcps: ["jira"],
    requiredSkills: ["tech_spec_skill"],
    model: "gpt-5.4",
    reasoningEffort: "medium",
    agentType: "standard",
    createdAt: "2026-05-20T00:00:00.000Z",
    updatedAt: "2026-05-21T00:00:00.000Z",
    ...overrides,
  };
}

describe("CreateWorkflowStepPage", () => {
  beforeEach(() => {
    mocks.createGatewayBundle.mockReset();
    vi.spyOn(window, "alert").mockImplementation(() => undefined);
  });

  it("saves step definition artifact bindings", async () => {
    const gatewayBundle = {
      workflowEngineGateway: {
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
        saveStepDefinition: vi.fn().mockResolvedValue(
          buildStep({
            stepType: "custom_step",
            name: "Custom Step",
            description: "Custom desc",
            inputArtifactDefinitions: ["business_summary_artifact"],
            outputArtifactDefinitions: ["tech_spec_artifact"],
          })
        ),
      },
    };
    mocks.createGatewayBundle.mockReturnValue(gatewayBundle);

    render(<CreateWorkflowStepPage />);

    fireEvent.change(screen.getByLabelText("Step key"), {
      target: { value: "custom_step" },
    });
    fireEvent.change(screen.getByLabelText("Display name"), {
      target: { value: "Custom Step" },
    });
    fireEvent.change(screen.getByLabelText("Description"), {
      target: { value: "Custom desc" },
    });
    fireEvent.change(screen.getByLabelText("Required MCPs"), {
      target: { value: "jira" },
    });
    fireEvent.change(screen.getByLabelText("Required skills"), {
      target: { value: "skill_a, skill_b" },
    });
    fireEvent.change(screen.getByLabelText("Prompt base"), {
      target: { value: "Drive the custom step execution." },
    });
    fireEvent.change(screen.getByLabelText("Reasoning effort"), {
      target: { value: "medium" },
    });
    fireEvent.click(screen.getAllByRole("button", { name: "Add artifact" })[0]);
    fireEvent.click(screen.getAllByRole("button", { name: "Add artifact" })[1]);
    fireEvent.click(screen.getByRole("button", { name: "Save step" }));

    await waitFor(() => {
      expect(gatewayBundle.workflowEngineGateway.saveStepDefinition).toHaveBeenCalledWith(
        expect.objectContaining({
          stepType: "custom_step",
          name: "Custom Step",
          description: "Custom desc",
          promptBase: "Drive the custom step execution.",
          requiredMcps: ["jira"],
          requiredSkills: ["skill_a", "skill_b"],
          model: "gpt-5.4",
          reasoningEffort: "medium",
          inputArtifactDefinitions: ["business_summary_artifact"],
          outputArtifactDefinitions: ["tech_spec_artifact"],
        })
      );
    });
  });
});
