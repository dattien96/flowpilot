import test from "node:test";
import assert from "node:assert/strict";
import { isFocusedChildLive, liveAgentRunsWithoutVisibleCard } from "./Timeline";
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

// The "back to main agent" crumb's green pulsing "live child run" badge must
// track the focused child's OWN status, not just "a child happens to be
// focused" — before this it kept pulsing green forever even after the child
// actually completed, disagreeing with the static "done" card the same run
// already shows in the Agents sidebar (AgentsPanel's active/closed split).
test("isFocusedChildLive is true while the focused child is still running or waiting", () => {
  assert.equal(isFocusedChildLive(run("child-1", "running")), true);
  assert.equal(isFocusedChildLive(run("child-1", "waiting_approval")), true);
  assert.equal(isFocusedChildLive(run("child-1", "waiting_question")), true);
});

test("isFocusedChildLive is false once the focused child reaches a terminal status", () => {
  assert.equal(isFocusedChildLive(run("child-1", "completed")), false);
  assert.equal(isFocusedChildLive(run("child-1", "failed")), false);
  assert.equal(isFocusedChildLive(run("child-1", "cancelled")), false);
});

test("isFocusedChildLive defaults to live when the run summary has not loaded yet", () => {
  assert.equal(isFocusedChildLive(undefined), true);
});
