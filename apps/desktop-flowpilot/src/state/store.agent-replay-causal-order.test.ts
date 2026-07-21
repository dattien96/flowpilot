import test from "node:test";
import assert from "node:assert/strict";
import type { ProviderEventDTO, ProviderKey } from "../types/contract";
import { orderHistoryReplayEvents } from "./store";

function event(
  providerKey: ProviderKey,
  seq: number,
  type: ProviderEventDTO["type"],
  occurredAt: string,
): ProviderEventDTO {
  return {
    id: `event-${seq}`,
    seq,
    type,
    workflowRunId: "run-1264",
    providerSessionId: "session-1",
    providerKey,
    occurredAt,
    agentName: type === "agent_spawned_by_user" ? `agent-${seq}` : undefined,
    childRunId: type === "agent_spawned_by_user" ? `child-${seq}` : undefined,
    text: type === "message_completed" ? "Both reviewers approved with no conflicts." : undefined,
  } as unknown as ProviderEventDTO;
}

for (const providerKey of ["codex", "claude", "grok"] as const satisfies ProviderKey[]) {
  test(`history replay keeps ${providerKey} agent cards ahead of a fallback-timestamp result`, () => {
    const ordered = orderHistoryReplayEvents([
      event(providerKey, 1, "turn_started", "2026-07-18T14:46:22.260537Z"),
      event(providerKey, 5, "agent_spawned_by_user", "2026-07-18T14:46:24.311119Z"),
      event(providerKey, 6, "agent_spawned_by_user", "2026-07-18T14:47:10.991123Z"),
      event(providerKey, 7, "agent_spawned_by_user", "2026-07-18T14:47:11.507457Z"),
      event(providerKey, 8, "message_completed", "2026-07-18T14:46:22.260537Z"),
    ]);

    assert.deepEqual(ordered.map((item) => item.seq), [1, 5, 6, 7, 8]);
  });
}
