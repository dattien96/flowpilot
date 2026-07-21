import test from "node:test";
import assert from "node:assert/strict";
import { deriveOrchestrationRunStatus, settleCompletedFlowTimeline, type TimelineItem } from "./store";
import type { AgentGraphSnapshot, ProviderKey } from "../types/contract";

for (const providerKey of ["codex", "claude", "grok"] as const satisfies ProviderKey[]) {
  test(`done flow settles terminal timeline residue for ${providerKey}`, () => {
    const snapshot: AgentGraphSnapshot = {
      parentRunId: "run-flow",
      runs: [
        {
          runId: "child-1",
          agentName: "reviewer",
          role: "reviewer",
          status: "running",
          parentRunId: "run-flow",
          createdAt: "2026-07-19T14:00:00Z",
          providerKey,
        },
      ],
      edges: [],
      busMessages: [],
      loopState: { status: "done", round: 1, roundCap: 3 },
    };
    const timeline: TimelineItem[] = [
      { kind: "tool", id: "tool-1", toolName: "flowpilot__submit_review_outcome", status: "running" },
      { kind: "thinking", id: "thinking-1", text: "Thinking..." },
    ];

    assert.equal(deriveOrchestrationRunStatus("running", snapshot), "completed");
    assert.deepEqual(settleCompletedFlowTimeline(timeline), [
      { kind: "tool", id: "tool-1", toolName: "flowpilot__submit_review_outcome", status: "success" },
    ]);
  });
}
