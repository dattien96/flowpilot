import test from "node:test";
import assert from "node:assert/strict";
import { liveAgentRunsWithoutVisibleCard } from "./Timeline";
import type { AgentRunSummary } from "@/types/contract";
import type { TimelineItem } from "@/state/store";

function run(runId: string, status: AgentRunSummary["status"]): AgentRunSummary {
  return {
    runId,
    agentName: "coder",
    role: "coder",
    status,
    createdAt: "2026-07-19T14:00:00Z",
  };
}

test("live agent banner excludes a child already represented by its lifecycle card", () => {
  const timeline: TimelineItem[] = [
    { kind: "agent", id: "agent-card-1", agentName: "coder", childRunId: "child-visible" },
  ];

  assert.deepEqual(
    liveAgentRunsWithoutVisibleCard(
      [run("child-visible", "running"), run("child-paged-out", "waiting_approval"), run("child-done", "completed")],
      timeline,
    ).map((item) => item.runId),
    ["child-paged-out"],
  );
});
