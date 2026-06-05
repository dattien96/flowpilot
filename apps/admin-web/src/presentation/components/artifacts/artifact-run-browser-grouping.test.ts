import { describe, expect, it } from "vitest";

import { groupArtifactsForDisplay } from "./artifact-run-browser-grouping";

describe("groupArtifactsForDisplay", () => {
  it("groups artifacts by project, storage type, workflow run, and workflow step", () => {
    const grouped = groupArtifactsForDisplay([
      {
        projectId: "project-1",
        projectName: "Project One",
        storageType: "supabase",
        workflowRunId: "run-1",
        workflowRunStepId: "step-1",
        createdAt: "2026-06-05T01:00:00.000Z",
      },
      {
        projectId: "project-1",
        projectName: "Project One",
        storageType: "supabase",
        workflowRunId: "run-1",
        workflowRunStepId: "step-2",
        createdAt: "2026-06-05T00:00:00.000Z",
      },
      {
        projectId: "project-1",
        projectName: "Project One",
        storageType: "local_only",
        workflowRunId: "run-2",
        workflowRunStepId: "draft",
        createdAt: "2026-06-05T02:00:00.000Z",
      },
    ]);

    expect(grouped).toHaveLength(1);
    expect(grouped[0]?.storageTypes.map((storageType) => storageType.storageType)).toEqual([
      "local_only",
      "supabase",
    ]);
    expect(grouped[0]?.storageTypes[1]?.workflowRuns[0]?.workflowRunId).toBe("run-1");
    expect(
      grouped[0]?.storageTypes[1]?.workflowRuns[0]?.workflowSteps.map((workflowStep) => workflowStep.workflowStepId),
    ).toEqual(["step-1", "step-2"]);
  });
});
