import test from "node:test";
import assert from "node:assert/strict";
import type { ProviderEventDTO, ProviderKey } from "../types/contract";
import { orderHistoryReplayEvents, settleCompletedFlowTimeline, type TimelineItem } from "./store";

function event(
  providerKey: ProviderKey,
  seq: number,
  occurredAt: string,
  type: ProviderEventDTO["type"],
): ProviderEventDTO {
  return {
    id: `evt-${seq}`,
    seq,
    type,
    workflowRunId: "run-2383",
    providerKey,
    occurredAt,
  } as ProviderEventDTO;
}

for (const providerKey of ["codex", "claude", "grok"] as const satisfies ProviderKey[]) {
  test(`history replay orders recovered lifecycle events for ${providerKey}`, () => {
    const ordered = orderHistoryReplayEvents([
      event(providerKey, 12, "2026-07-19T14:37:38.334334Z", "message_completed"),
      event(providerKey, 7, "2026-07-19T14:37:40.243014Z", "agent_spawned_by_user"),
      event(providerKey, 17, "2026-07-19T14:41:39.251364Z", "agent_result_injected"),
      event(providerKey, 8, "2026-07-19T14:38:54.766468Z", "agent_spawned_by_user"),
      event(providerKey, 18, "2026-07-19T14:40:22.833513Z", "agent_result_injected"),
    ]);

    assert.deepEqual(ordered.map((item) => item.seq), [12, 7, 8, 18, 17]);
  });

  test(`completed replay clears every running tool for ${providerKey}`, () => {
    const timeline: TimelineItem[] = [
      { kind: "tool", id: "search", toolName: "search_tool", status: "running" },
      { kind: "tool", id: "review", toolName: "flowpilot__submit_review_outcome", status: "running" },
      { kind: "thinking", id: "thinking", text: "Thinking..." },
    ];

    assert.deepEqual(settleCompletedFlowTimeline(timeline), [
      { kind: "tool", id: "search", toolName: "search_tool", status: "success" },
      { kind: "tool", id: "review", toolName: "flowpilot__submit_review_outcome", status: "success" },
    ]);
  });
}
