import { fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import type { AiRun, AiRunSummary } from "@/domain/model/entity/ai-orchestration";

import { AiRunsContent } from "./ai-runs";

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
  };
});

const runs: AiRun[] = [
  {
    id: "ai-run-001",
    projectId: "project_mobile_replatform",
    runType: "tech_spec_generation",
    inputPayload: { sourceArtifactRunId: "artifact-run-001" },
    outputPayload: { artifactRunId: "artifact-run-002" },
    modelName: "claude-sonnet-4",
    triggeredBy: "user-1",
    status: "success",
    errorMessage: null,
    tokensInput: 1000,
    tokensOutput: 500,
    costUsd: 0.75,
    promptTemplateId: "tmpl-1",
    workflowRunId: "wf-run-001",
    workflowRunStepId: "wf-step-001",
    artifactRunId: "artifact-run-002",
    createdAt: "2026-05-21T09:00:00.000Z",
    completedAt: "2026-05-21T09:01:00.000Z",
  },
  {
    id: "ai-run-002",
    projectId: "project_mobile_replatform",
    runType: "coding_plan_generation",
    inputPayload: { sourceArtifactRunId: "artifact-run-002" },
    outputPayload: {},
    modelName: "codex-5.5",
    triggeredBy: "user-1",
    status: "failed",
    errorMessage: "Provider timeout.",
    tokensInput: 2000,
    tokensOutput: 0,
    costUsd: 0.25,
    promptTemplateId: "tmpl-2",
    workflowRunId: "wf-run-002",
    workflowRunStepId: "wf-step-002",
    artifactRunId: null,
    createdAt: "2026-05-21T10:00:00.000Z",
    completedAt: "2026-05-21T10:02:00.000Z",
  },
];

const summary: AiRunSummary = {
  totalRuns: 2,
  runningRuns: 0,
  successfulRuns: 1,
  failedRuns: 1,
  totalInputTokens: 3000,
  totalOutputTokens: 500,
  totalCostUsd: 1,
};

describe("AiRunsContent", () => {
  it("renders ai run metrics and lineage ids", () => {
    render(<AiRunsContent runs={runs} summary={summary} />);

    expect(screen.getByText("AI Runs")).toBeInTheDocument();
    expect(screen.getByText("tech_spec_generation")).toBeInTheDocument();
    expect(screen.getByText("wf-run-001")).toBeInTheDocument();
    expect(screen.getByText("$1.00")).toBeInTheDocument();
  });

  it("filters the execution ledger by status", () => {
    render(<AiRunsContent runs={runs} summary={summary} />);

    fireEvent.click(screen.getByRole("button", { name: "Failed (1)" }));

    expect(screen.getByText("coding_plan_generation")).toBeInTheDocument();
    expect(screen.queryByText("tech_spec_generation")).not.toBeInTheDocument();
    expect(screen.getByText("Provider timeout.")).toBeInTheDocument();
  });

  it("filters the execution ledger by model", () => {
    render(<AiRunsContent runs={runs} summary={summary} />);

    fireEvent.change(screen.getByDisplayValue("All models"), {
      target: { value: "claude-sonnet-4" },
    });

    expect(screen.getByText("tech_spec_generation")).toBeInTheDocument();
    expect(screen.queryByText("coding_plan_generation")).not.toBeInTheDocument();
  });
});
