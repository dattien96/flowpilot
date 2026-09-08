import assert from "node:assert/strict";
import test from "node:test";
import { detectVibeEntry, isCodingPlanCPPath, workspaceRelPathFromFile } from "./workingMode";

test("detectVibeEntry routes CP path to vibe-cp-ingest", () => {
  const path = "requirements/07-Coding-Plan/inprogress/CP-60-Vibe-Working-Mode.md";
  assert.equal(isCodingPlanCPPath(path), true);
  const got = detectVibeEntry(path);
  assert.equal(got.flowRef, "vibe-cp-ingest");
  assert.equal(got.sourceDocId, path);
});

test("detectVibeEntry routes raw idea to vibe-ingest", () => {
  const got = detectVibeEntry("fix login timeout");
  assert.equal(got.flowRef, "vibe-ingest");
  assert.equal(got.sourceDocId, "fix login timeout");
});

test("workspaceRelPathFromFile strips cwd prefix", () => {
  const rel = workspaceRelPathFromFile(
    { name: "CP-60.md", path: "C:/working/flowpilot/requirements/07-Coding-Plan/inprogress/CP-60.md" },
    "C:/working/flowpilot",
  );
  assert.equal(rel, "requirements/07-Coding-Plan/inprogress/CP-60.md");
});
