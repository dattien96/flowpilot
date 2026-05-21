import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import type { Feature } from "@/domain/model/entity/feature";
import type { Project } from "@/domain/model/entity/project";

import { FeaturesContent, FeaturesPage } from "./features";

const mocks = vi.hoisted(() => ({
  pathname: "/features",
}));

vi.mock("@/components/common/page-frame", () => ({
  PageFrame: ({
    children,
    description,
    title,
  }: {
    children?: ReactNode;
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
    }) => <a href={to.replace("$featureId", params?.featureId ?? "")}>{children}</a>,
    Outlet: () => <div>child route</div>,
    createFileRoute: () => () => ({
      useLoaderData: () => ({ features, projects }),
    }),
    useLocation: () => ({ pathname: mocks.pathname }),
  };
});

const projects: Project[] = [
  {
    id: "project-alpha",
    name: "Alpha",
    description: "Alpha project",
    platform: "web",
    repositoryUrl: "https://example.com/repo.git",
    directoryPath: null,
    ownerId: null,
    status: "active",
    artifactStoragePreference: "supabase",
    createdBy: "demo-user",
    createdAt: "2026-05-20T00:00:00.000Z",
    updatedAt: "2026-05-20T00:00:00.000Z",
  },
];

const features: Feature[] = [
  {
    id: "feature-alpha",
    projectId: "project-alpha",
    title: "Alpha feature",
    businessGoal: "Ship alpha planning",
    userProblem: "Users need a fast path to feature detail.",
    expectedFlow: "Open feature detail and start a workflow.",
    acceptanceCriteria: "Feature card navigation works.",
    priority: "high",
    status: "active",
    ownerId: "demo-user",
    createdAt: "2026-05-20T00:00:00.000Z",
    updatedAt: "2026-05-20T00:00:00.000Z",
  },
];

describe("FeaturesPage", () => {
  it("renders the child route for nested feature paths", () => {
    mocks.pathname = "/features/feature-alpha";

    render(<FeaturesPage />);

    expect(screen.getByText("child route")).toBeInTheDocument();
    expect(screen.queryByText("Alpha feature")).not.toBeInTheDocument();
  });

  it("renders feature cards with detail links", () => {
    mocks.pathname = "/features";

    render(<FeaturesContent features={features} projects={projects} />);

    const featureLink = screen.getByRole("link", { name: /alpha feature/i });
    expect(featureLink).toHaveAttribute("href", "/features/feature-alpha");
    expect(screen.getByText("Alpha")).toBeInTheDocument();
  });
});
