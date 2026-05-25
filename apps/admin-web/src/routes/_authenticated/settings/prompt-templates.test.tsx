import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import type { AiPromptTemplate } from "@/domain/model/entity/ai-orchestration";

import { PromptTemplatesContent, PromptTemplatesPage } from "./prompt-templates";

const mocks = vi.hoisted(() => ({
  pathname: "/settings/prompt-templates",
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

vi.mock("@/data/repository/browser-factory", () => ({
  createGatewayBundle: () => ({
    aiOrchestrationGateway: {},
  }),
}));

vi.mock("@/domain/usecase/ai-orchestration/save-prompt-template-usecase", () => ({
  SavePromptTemplateUseCase: class {
    execute = vi.fn();
  },
}));

vi.mock("@tanstack/react-router", async () => {
  const actual = await vi.importActual<typeof import("@tanstack/react-router")>(
    "@tanstack/react-router"
  );
  return {
    ...actual,
    Link: ({ children }: { children: ReactNode }) => <>{children}</>,
    Outlet: () => <div>child route</div>,
    createFileRoute: () => () => ({
      useLoaderData: () => ({ templates }),
    }),
    useLocation: () => ({ pathname: mocks.pathname }),
  };
});

const templates: AiPromptTemplate[] = [
  {
    id: "tmpl-1",
    projectId: null,
    stepType: "tech_spec",
    name: "Global Tech Spec",
    description: "Default generation prompt.",
    inputSchema: {},
    outputSchema: {},
    templateContent: "# Task\nGenerate a tech spec.",
    providerPreference: "claude",
    modelPreference: "claude-sonnet-4",
    version: 1,
    status: "active",
    createdBy: "user-1",
    createdAt: "2026-05-21T09:00:00.000Z",
    updatedAt: "2026-05-21T09:00:00.000Z",
  },
];

describe("PromptTemplatesContent", () => {
  it("renders the child route for nested prompt-template paths", () => {
    mocks.pathname = "/settings/prompt-templates/create";

    render(<PromptTemplatesPage />);

    expect(screen.getByText("child route")).toBeInTheDocument();
    expect(screen.queryByText("Versioned template catalog")).not.toBeInTheDocument();
  });

  it("renders prompt template inventory", () => {
    mocks.pathname = "/settings/prompt-templates";

    render(<PromptTemplatesContent initialTemplates={templates} onSave={vi.fn()} />);

    expect(screen.getByText("Prompt Templates")).toBeInTheDocument();
    expect(screen.getByText("Global Tech Spec")).toBeInTheDocument();
    expect(screen.getByText("Versioned template catalog")).toBeInTheDocument();
    expect(screen.getByText("1 global / 0 project")).toBeInTheDocument();
    const templateRow = screen.getByText("Global Tech Spec").closest("details");
    expect(templateRow).not.toBeNull();
    expect(templateRow).not.toHaveAttribute("open");
    expect(screen.getByText("EXPAND")).toBeInTheDocument();
  });

  it("saves an existing template in the registry", async () => {
    const onSave = vi.fn().mockResolvedValue({
      ...templates[0],
      name: "Updated Tech Spec",
    });

    render(<PromptTemplatesContent initialTemplates={templates} onSave={onSave} />);

    fireEvent.click(screen.getByText("EXPAND"));
    await waitFor(() => {
      expect(screen.getByText("COLLAPSE")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByDisplayValue("Global Tech Spec"), {
      target: { value: "Updated Tech Spec" },
    });

    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(onSave).toHaveBeenCalledWith(
        expect.objectContaining({
          id: "tmpl-1",
          name: "Updated Tech Spec",
          stepType: "tech_spec",
        })
      );
    });

    expect(screen.getByText("Saved Updated Tech Spec (v1).")).toBeInTheDocument();
  });
});
