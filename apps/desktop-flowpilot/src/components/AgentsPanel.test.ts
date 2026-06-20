import test from "node:test";
import assert from "node:assert/strict";
import { formatDependencyLabels } from "./agentDependencies";
import type { AgentRunSummary } from "@/types/contract";

const runs: AgentRunSummary[] = [
  {
    runId: "run-reviewer",
    agentName: "reviewer",
    role: "reviewer",
    status: "completed",
    createdAt: "2026-06-20T00:00:00Z",
  },
  {
    runId: "run-tester",
    agentName: "tester",
    role: "tester",
    status: "starting",
    createdAt: "2026-06-20T00:00:01Z",
  },
];

test("formatDependencyLabels resolves dependency run ids to agent names", () => {
  assert.deepEqual(formatDependencyLabels(["run-reviewer", "missing-run"], runs), ["reviewer", "missing-run"]);
});
