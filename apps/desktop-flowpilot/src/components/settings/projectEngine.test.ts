import test from "node:test";
import assert from "node:assert/strict";
import { summarizeProjectEngineInit, type ProjectEngineInitResult } from "./projectEngine";

function initResult(overrides: Partial<ProjectEngineInitResult> & {
  installed?: string[];
  skippedPaths?: string[];
  errors?: string[];
}): ProjectEngineInitResult {
  const { installed = [], skippedPaths = [], errors = [], ...rest } = overrides;
  return {
    trigger: "manual",
    status: "success",
    skipped: false,
    attemptedAt: "t",
    completedAt: "t",
    workingDirectory: "/x",
    install: { installedPaths: installed, skippedPaths, errors },
    steps: [],
    ...rest,
  };
}

test("summarizeProjectEngineInit reads a fully-current re-init as up to date", () => {
  const summary = summarizeProjectEngineInit(
    initResult({ skippedPaths: ["a/SKILL.md", "b/SKILL.md", "c/SKILL.md"] }),
  );
  assert.equal(summary, "Completed: all 3 file(s) already current.");
});

test("summarizeProjectEngineInit keeps install counts visible when files were written", () => {
  const summary = summarizeProjectEngineInit(
    initResult({ installed: ["a/SKILL.md"], skippedPaths: ["b/SKILL.md"] }),
  );
  assert.equal(summary, "Completed: 1 installed, 1 already current, 0 errors.");
});

test("summarizeProjectEngineInit surfaces errors instead of the up-to-date wording", () => {
  const summary = summarizeProjectEngineInit(
    initResult({ status: "partial", skippedPaths: ["a"], errors: ["write x: denied"] }),
  );
  assert.equal(summary, "Completed with warnings: 0 installed, 1 already current, 1 errors.");
});

test("summarizeProjectEngineInit reports a bind-skip distinctly", () => {
  const summary = summarizeProjectEngineInit(initResult({ status: "skipped", skipped: true }));
  assert.equal(summary, "Skipped: 0 installed, 0 already current, 0 errors.");
});
