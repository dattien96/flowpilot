import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { CreatePromptTemplatePage } from "./create";

const mocks = vi.hoisted(() => ({
  execute: vi.fn(),
  navigate: vi.fn(),
}));

vi.mock("@/data/repository/browser-factory", () => ({
  createGatewayBundle: () => ({
    aiOrchestrationGateway: {},
  }),
}));

vi.mock("@/domain/usecase/ai-orchestration/save-prompt-template-usecase", () => ({
  SavePromptTemplateUseCase: class {
    execute = mocks.execute;
  },
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
    Link: ({ children }: { children: ReactNode }) => <>{children}</>,
    createFileRoute: () => () => ({}),
    useNavigate: () => mocks.navigate,
  };
});

describe("CreatePromptTemplatePage", () => {
  it("creates a new prompt template and returns to the registry", async () => {
    mocks.execute.mockResolvedValue({ id: "tmpl-new" });
    mocks.navigate.mockResolvedValue(undefined);

    render(<CreatePromptTemplatePage />);

    fireEvent.change(screen.getByPlaceholderText("Global Tech Spec"), {
      target: { value: "Project Coding Plan" },
    });
    fireEvent.change(screen.getByPlaceholderText("tech_spec"), {
      target: { value: "make_plan_coding" },
    });

    fireEvent.click(screen.getByRole("button", { name: "Create Template" }));

    await waitFor(() => {
      expect(mocks.execute).toHaveBeenCalledWith(
        expect.objectContaining({
          name: "Project Coding Plan",
          stepType: "make_plan_coding",
        })
      );
    });

    expect(mocks.navigate).toHaveBeenCalledWith({ to: "/settings/prompt-templates" });
  });
});
