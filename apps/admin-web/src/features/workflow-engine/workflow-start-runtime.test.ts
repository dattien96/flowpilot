import { describe, expect, it } from "vitest";

import {
  buildWorkflowStepFollowUpPrompt,
  buildWorkflowStepPrompt,
  resolveProviderKeyFromModel,
} from "./workflow-start-runtime";

describe("workflow-start-runtime", () => {
  it("routes supported models to the correct local provider", () => {
    expect(resolveProviderKeyFromModel("gpt-5.5")).toBe("codex");
    expect(resolveProviderKeyFromModel("gemini-pro")).toBe("gemini");
    expect(resolveProviderKeyFromModel("claude-sonnet")).toBe("claude");
  });

  it("builds prompts with runtime context and artifact paths", () => {
    const prompt = buildWorkflowStepPrompt({
      beginPrompt: "Create the implementation plan.",
      inputArtifactPaths: ["C:/repo/.flowpilot/artifacts/input.md"],
      model: "gpt-5.5",
      outputArtifactPaths: ["C:/repo/.flowpilot/artifacts/output.md"],
      promptBase: "Turn the requirement into a concrete implementation plan.",
      stepType: "make_plan_coding",
      subagent: "planner-agent",
      teamRole: "tech_lead",
      workingDirectory: "C:/repo",
      requiredSkills: ["planner-skill"],
    });

    expect(prompt).toContain("Step type: make_plan_coding");
    expect(prompt).toContain("Model: gpt-5.5");
    expect(prompt).toContain("Working directory: C:/repo");
    expect(prompt).toContain("tech_lead");
    expect(prompt).toContain("planner-agent");
    expect(prompt).toContain("C:/repo/.flowpilot/artifacts/input.md");
    expect(prompt).toContain("C:/repo/.flowpilot/artifacts/output.md");
  });

  it("builds follow-up prompts without replaying the initial begin prompt", () => {
    const prompt = buildWorkflowStepFollowUpPrompt({
      followUpPrompt: "Make the artifact shorter.",
      inputArtifactPaths: ["C:/repo/.flowpilot/artifacts/input.md"],
      outputArtifactPaths: ["C:/repo/.flowpilot/artifacts/output.md"],
      workingDirectory: "C:/repo",
    });

    expect(prompt).toContain("Make the artifact shorter.");
    expect(prompt).toContain("Artifacts To Review");
    expect(prompt).toContain("C:/repo/.flowpilot/artifacts/output.md");
    expect(prompt).not.toContain("## Begin Prompt");
    expect(prompt).not.toContain("## Prompt Base");
  });
});
