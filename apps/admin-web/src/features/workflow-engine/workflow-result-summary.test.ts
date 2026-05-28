import { describe, expect, it } from "vitest";

import {
  appendSyntheticResultSummarySteps,
  buildResultSummaryPrompt,
  RESULT_SUMMARY_STEP_NAME,
  RESULT_SUMMARY_STEP_TYPE,
  shouldAppendResultSummaryStep,
} from "./workflow-result-summary";

describe("workflow-result-summary", () => {
  it("appends the built-in summary step when the workflow has at least two main steps", () => {
    expect(shouldAppendResultSummaryStep([{ step_type: "a" }])).toBe(false);
    expect(shouldAppendResultSummaryStep([{ step_type: "a" }, { step_type: "b" }])).toBe(true);
    expect(shouldAppendResultSummaryStep([{ step_type: "a" }, { step_type: "b" }, { step_type: "c" }])).toBe(true);
  });

  it("does not append the built-in summary step if the last step is already a summary step", () => {
    expect(shouldAppendResultSummaryStep([{ step_type: "a" }, { step_type: "result_summary" }])).toBe(false);
  });

  it("builds a final summary prompt from completed step outputs", () => {
    const prompt = buildResultSummaryPrompt({
      beginPrompt: "Create a business and implementation package.",
      workflowName: "Launch Prep",
      workingDirectory: "C:/repo",
      steps: [
        {
          stepName: "Business Idea",
          stepType: "business_idea",
          outputMarkdown: "# Business Idea\n\nA collaboration app for teams.",
          artifactOutputPaths: ["C:/repo/.flowpilot/artifacts/project-1/run-1/business_idea/BusinessIdea.md"],
          startedAt: "2026-05-28T01:00:00.000Z",
          completedAt: "2026-05-28T01:01:00.000Z",
        },
        {
          stepName: "Tech Spec",
          stepType: "tech_spec",
          outputMarkdown: "# Tech Spec\n\nUse React and Go services.",
          artifactOutputPaths: [],
          startedAt: "2026-05-28T01:01:00.000Z",
          completedAt: "2026-05-28T01:02:00.000Z",
        },
      ],
    });

    expect(prompt).toContain("Create a final workflow-level summary");
    expect(prompt).toContain("Launch Prep");
    expect(prompt).toContain("Create a business and implementation package.");
    expect(prompt).toContain("### Step 1: Business Idea");
    expect(prompt).toContain("### Step 2: Tech Spec");
    expect(prompt).toContain("C:/repo/.flowpilot/artifacts/project-1/run-1/business_idea/BusinessIdea.md");
    expect(prompt).toContain("A collaboration app for teams.");
    expect(prompt).toContain("Do not summarize this summary step itself.");
  });

  it("merges the runtime-generated result summary step into the visible workflow steps", () => {
    const merged = appendSyntheticResultSummarySteps({
      baseSteps: [
        {
          id: "step-1",
          workflowRunId: "run-1",
          stepKey: "business_idea",
          stepName: "Business Idea",
          stepType: "ai_mock",
          status: "completed",
          sequenceIndex: 0,
          outputId: null,
          startedAt: null,
          completedAt: null,
          errorMessage: null,
        },
      ],
      engineSteps: [
        {
          id: "engine-summary-1",
          workflowRunId: "run-1",
          workflowStepId: null,
          executionOrderIndex: 1,
          stepType: RESULT_SUMMARY_STEP_TYPE,
          status: "DONE",
          artifactId: null,
          artifactRunId: null,
          promptCacheId: null,
          rejectionNote: null,
          retryCount: 0,
          startedAt: "2026-05-28T01:03:00.000Z",
          finishedAt: "2026-05-28T01:04:00.000Z",
          errorMessage: null,
        },
      ],
      runId: "run-1",
    });

    expect(merged).toHaveLength(2);
    expect(merged[1]).toMatchObject({
      id: "engine-summary-1",
      stepKey: RESULT_SUMMARY_STEP_TYPE,
      stepName: RESULT_SUMMARY_STEP_NAME,
      stepType: "ai_mock",
      status: "completed",
      sequenceIndex: 1,
    });
  });
});
