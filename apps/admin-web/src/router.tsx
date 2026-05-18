import {
  Outlet,
  createRootRoute,
  createRouter,
  createRoute,
  redirect,
} from "@tanstack/react-router";
import type { ReactElement } from "react";

import {
  AiRunsPage,
  ArtifactManagementPage,
  ArtifactMemoryPage,
  ApprovalsPage,
  DashboardPage,
  FeaturesPage,
  LoginPage,
  OutputsPage,
  ProjectDetailPage,
  ProjectsPage,
  PromptTemplatesPage,
  SettingsPage,
  WorkflowDefinitionsPage,
  WorkflowRunsPage,
} from "@/routes/pages";
import { getOptionalAdminSession, requireAdminSession } from "@/features/auth/require-auth";

function RootLayout() {
  return <Outlet />;
}

function routeTo(path: string) {
  return path as never;
}

const rootRoute = createRootRoute({
  component: RootLayout,
});

const homeRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  beforeLoad: async () => {
    const session = await getOptionalAdminSession();
    throw redirect({ to: routeTo(session ? "/dashboard" : "/login") });
  },
  component: () => null,
});

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/login",
  component: LoginPage,
});

function protectedRoute(path: string, component: () => ReactElement) {
  return createRoute({
    getParentRoute: () => rootRoute,
    path,
    beforeLoad: requireAdminSession,
    component,
  });
}

const dashboardRoute = protectedRoute("/dashboard", DashboardPage);
const projectsRoute = protectedRoute("/projects", ProjectsPage);
const projectDetailRoute = protectedRoute("/projects/$projectId", ProjectDetailPage);
const featuresRoute = protectedRoute("/features", FeaturesPage);
const workflowDefinitionsRoute = protectedRoute(
  "/workflow-definitions",
  WorkflowDefinitionsPage,
);
const workflowRunsRoute = protectedRoute("/workflow-runs", WorkflowRunsPage);
const approvalsRoute = protectedRoute("/approvals", ApprovalsPage);
const outputsRoute = protectedRoute("/outputs", OutputsPage);
const artifactManagementRoute = protectedRoute("/artifacts", ArtifactManagementPage);
const artifactMemoryRoute = protectedRoute("/artifacts/$artifactId", ArtifactMemoryPage);
const aiRunsRoute = protectedRoute("/ai-runs", AiRunsPage);
const settingsRoute = protectedRoute("/settings", SettingsPage);
const promptTemplatesRoute = protectedRoute(
  "/settings/prompt-templates",
  PromptTemplatesPage,
);

const routeTree = rootRoute.addChildren([
  homeRoute,
  loginRoute,
  dashboardRoute,
  projectsRoute,
  projectDetailRoute,
  featuresRoute,
  workflowDefinitionsRoute,
  workflowRunsRoute,
  approvalsRoute,
  outputsRoute,
  artifactManagementRoute,
  artifactMemoryRoute,
  aiRunsRoute,
  settingsRoute,
  promptTemplatesRoute,
]);

export const router = createRouter({
  routeTree,
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
