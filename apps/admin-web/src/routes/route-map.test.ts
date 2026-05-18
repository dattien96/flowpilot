import { describe, expect, it } from "vitest";

import { router } from "@/router";

describe("CP-01 route tree", () => {
  it("registers the authenticated project child routes and settings routes", () => {
    const routePaths = Object.keys(router.routesByPath);

    expect(routePaths).toContain("/login");
    expect(routePaths).toContain("/dashboard");
    expect(routePaths).toContain("/projects");
    expect(routePaths).toContain("/teams");
    expect(routePaths).toContain("/projects/$projectId/business-logic");
    expect(routePaths).toContain("/projects/$projectId/tech-specs");
    expect(routePaths).toContain("/projects/$projectId/coding-plan");
    expect(routePaths).toContain("/projects/$projectId/master-schedule");
    expect(routePaths).toContain("/projects/$projectId/tasks");
    expect(routePaths).toContain("/projects/$projectId/members");
    expect(routePaths).toContain("/projects/$projectId/workflows");
    expect(routePaths).toContain("/projects/$projectId/settings");
    expect(routePaths).toContain("/ai-runs");
    expect(routePaths).toContain("/settings/integrations");
    expect(routePaths).toContain("/settings/prompt-templates");
  });
});
