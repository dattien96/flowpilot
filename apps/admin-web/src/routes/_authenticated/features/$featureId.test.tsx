import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { FeatureDetailContent } from "./$featureId";

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
    "@tanstack/react-router",
  );

  return {
    ...actual,
    Link: ({
      children,
      params,
      to,
    }: {
      children: ReactNode;
      params?: Record<string, string>;
      to: string;
    }) => (
      <a
        href={to
          .replace("$featureId", params?.featureId ?? "")
          .replace("$projectId", params?.projectId ?? "")}
      >
        {children}
      </a>
    ),
    createFileRoute: () => () => ({
      useLoaderData: () => ({ detail: null }),
    }),
  };
});

function renderSubject() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

  const onStartWorkflow = vi.fn().mockResolvedValue({
    id: "run-1",
    workflowId: "workflow-1",
    projectId: "project-alpha",
    status: "PENDING",
    provider: null,
    model: null,
    yoloMode: false,
    startedBy: "demo-user",
    startedAt: "2026-05-20T00:00:00.000Z",
    finishedAt: null,
    errorMessage: null,
  });

  const detail = {
    feature: {
      id: "feature-alpha",
      projectId: "project-alpha",
      title: "Alpha feature",
      businessGoal: "Ship alpha planning",
      userProblem: "Users need a fast path to feature detail.",
      expectedFlow: "Open feature detail and start a workflow.",
      acceptanceCriteria: "Feature card navigation works.",
      priority: "high" as const,
      status: "active",
      ownerId: "demo-user",
      createdAt: "2026-05-20T00:00:00.000Z",
      updatedAt: "2026-05-20T00:00:00.000Z",
    },
    contexts: [
      {
        id: "context-1",
        projectId: "project-alpha",
        featureId: "feature-alpha",
        type: "manual_text" as const,
        title: "Primary context",
        rawContent: "Important product context.",
        summarizedContent: null,
        createdBy: "demo-user",
        createdAt: "2026-05-20T00:00:00.000Z",
      },
    ],
    runs: [],
    workflows: [
      {
        id: "workflow-1",
        projectId: null,
        name: "Feature to spec",
        description: "Demo workflow",
        isTemplate: false,
        providerOverride: null,
        modelOverride: null,
        createdBy: "demo-user",
        createdAt: "2026-05-20T00:00:00.000Z",
        updatedAt: "2026-05-20T00:00:00.000Z",
        steps: [],
      },
    ],
  };

  return {
    onStartWorkflow,
    ...render(
      <QueryClientProvider client={queryClient}>
        <FeatureDetailContent detail={detail} onStartWorkflow={onStartWorkflow} />
      </QueryClientProvider>,
    ),
  };
}

describe("FeatureDetailContent", () => {
  it("starts a workflow-engine run for the feature project", async () => {
    const { onStartWorkflow } = renderSubject();

    fireEvent.click(screen.getByRole("button", { name: "Start workflow run" }));

    await waitFor(() => {
      expect(onStartWorkflow).toHaveBeenCalledWith("workflow-1", "project-alpha");
    });

    expect(screen.getByText("Started workflow run run-1.")).toBeInTheDocument();
  });
});
