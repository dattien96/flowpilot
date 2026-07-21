import test from "node:test";
import assert from "node:assert/strict";
import type { ProviderEventDTO, ProviderKey } from "../types/contract";
import { orderHistoryReplayEvents } from "./store";

/**
 * run-24377: after resume, hub turn_started/message_completed from the durable
 * turn log often have empty occurredAt while agent_spawned_by_user carries child
 * wall-clock starts. The old sorter put timed events first → agent cards above
 * the original "fix bug" prompt.
 *
 * cross-provider-parity: Case 1 — same sorter for all providers.
 * additive-tests-only: new file only.
 */
function evt(
  providerKey: ProviderKey,
  seq: number,
  type: ProviderEventDTO["type"],
  occurredAt: string,
  extra: Partial<ProviderEventDTO> = {},
): ProviderEventDTO {
  return {
    id: `e-${seq}`,
    seq,
    type,
    workflowRunId: "run-24377",
    providerKey,
    occurredAt,
    ...extra,
  } as ProviderEventDTO;
}

for (const providerKey of ["codex", "claude", "grok"] as const satisfies ProviderKey[]) {
  test(`history replay keeps untimed hub prompt before timed agents (${providerKey})`, () => {
    const ordered = orderHistoryReplayEvents([
      evt(providerKey, 1, "turn_started", "", { prompt: "fix bug 1 + 1 != 2" }),
      evt(providerKey, 2, "agent_spawned_by_user", "2026-07-21T14:48:57.806588Z", {
        agentName: "coder",
        childRunId: "run-24382",
      }),
      evt(providerKey, 3, "agent_result_injected", "2026-07-21T14:50:00Z", {
        childRunId: "run-24382",
        finalMessage: "done",
      }),
      evt(providerKey, 4, "message_completed", "", { text: "Round 0: changes requested" }),
      evt(providerKey, 5, "turn_started", "", { prompt: "done rồi hả, trả lời ok or not." }),
    ]);

    assert.deepEqual(
      ordered.map((item) => item.seq),
      [1, 2, 3, 4, 5],
      "must preserve Seq when any occurredAt is missing — not dump timed agents first",
    );
    assert.equal(ordered[0].type, "turn_started");
    assert.equal(ordered[0].prompt, "fix bug 1 + 1 != 2");
  });
}
