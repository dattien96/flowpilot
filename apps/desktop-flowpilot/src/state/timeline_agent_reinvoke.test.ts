import test from "node:test";
import assert from "node:assert/strict";
import { applyTimelineEvent, type TimelineState } from "./timelineReducer";
import type { ProviderEventDTO } from "../types/contract";

/**
 * run-5695 / run-9034 — Review Loop lifecycle:reinvoke keeps one coder childRunId
 * across rounds. Server re-emits agent_spawned_by_user with a *new event id*
 * (BUG-Rnd2). Main chat must open a new card for each spawn event.
 *
 * Critical live constraint (wait:false flow children): agent_result_injected may
 * arrive only after gate settle — or historically never arrived on the parent
 * stream. Dedupe must NOT require finalMessage on the prior card.
 */

function state(timeline: TimelineState["timeline"] = []): TimelineState {
  return {
    status: "running",
    timeline,
    recoverable: false,
    pendingApprovals: [],
    pendingQuestions: [],
  };
}

function event(overrides: Partial<ProviderEventDTO>): ProviderEventDTO {
  return {
    id: "event-1",
    workflowRunId: "run-1",
    providerSessionId: "thread-1",
    providerKey: "grok",
    seq: 1,
    occurredAt: "2026-07-20T11:00:00Z",
    type: "turn_completed",
    finalMessage: "done",
    ...overrides,
  } as ProviderEventDTO;
}

test("reinvoke spawn with same childRunId opens a second agent card after the first completed", () => {
  let s = applyTimelineEvent(
    state(),
    event({ type: "agent_spawned_by_user", id: "spawn-r0", agentName: "coder", childRunId: "run-5700", seq: 1 }),
  );
  s = applyTimelineEvent(
    { ...state(), ...s, timeline: s.timeline ?? [] },
    event({
      type: "agent_result_injected",
      id: "result-r0",
      agentName: "coder",
      childRunId: "run-5700",
      finalMessage: "round-0 implement",
      seq: 2,
    }),
  );
  s = applyTimelineEvent(
    { ...state(), ...s, timeline: s.timeline ?? [] },
    event({ type: "agent_spawned_by_user", id: "spawn-r1", agentName: "coder", childRunId: "run-5700", seq: 3 }),
  );

  const agents = (s.timeline ?? []).filter((it) => it.kind === "agent");
  assert.equal(agents.length, 2, "reinvoke must push a second card, not reuse-only");
  assert.deepEqual(agents[0], {
    kind: "agent",
    id: "spawn-r0",
    agentName: "coder",
    childRunId: "run-5700",
    finalMessage: "round-0 implement",
  });
  assert.deepEqual(agents[1], {
    kind: "agent",
    id: "spawn-r1",
    agentName: "coder",
    childRunId: "run-5700",
  });
});

test("run-9034: second spawn opens a card even when live never injected a result yet (wait:false)", () => {
  // Mirrors production Review Loop: wait:false children historically never got
  // agent_result_injected on the parent until gate settle — and before that fix,
  // not at all. Round-2 reinvoke still must render a new card.
  let s = applyTimelineEvent(
    state(),
    event({ type: "agent_spawned_by_user", id: "spawn-r0", agentName: "coder", childRunId: "run-9039", seq: 1 }),
  );
  s = applyTimelineEvent(
    { ...state(), ...s, timeline: s.timeline ?? [] },
    event({ type: "agent_spawned_by_user", id: "spawn-r1", agentName: "coder", childRunId: "run-9039", seq: 2 }),
  );

  const agents = (s.timeline ?? []).filter((it) => it.kind === "agent");
  assert.equal(agents.length, 2, "wait:false reinvoke must not depend on finalMessage");
  assert.equal(agents[0].kind === "agent" && agents[0].id, "spawn-r0");
  assert.equal(agents[1].kind === "agent" && agents[1].id, "spawn-r1");
  assert.equal(agents[0].kind === "agent" && agents[0].finalMessage, undefined);
  assert.equal(agents[1].kind === "agent" && agents[1].finalMessage, undefined);
});

test("result after reinvoke binds to the latest open card, not the round-0 card", () => {
  let s = applyTimelineEvent(
    state(),
    event({ type: "agent_spawned_by_user", id: "spawn-r0", agentName: "coder", childRunId: "run-5700", seq: 1 }),
  );
  s = applyTimelineEvent(
    { ...state(), ...s, timeline: s.timeline ?? [] },
    event({
      type: "agent_result_injected",
      id: "result-r0",
      agentName: "coder",
      childRunId: "run-5700",
      finalMessage: "round-0 implement",
      seq: 2,
    }),
  );
  s = applyTimelineEvent(
    { ...state(), ...s, timeline: s.timeline ?? [] },
    event({ type: "agent_spawned_by_user", id: "spawn-r1", agentName: "coder", childRunId: "run-5700", seq: 3 }),
  );
  s = applyTimelineEvent(
    { ...state(), ...s, timeline: s.timeline ?? [] },
    event({
      type: "agent_result_injected",
      id: "result-r1",
      agentName: "coder",
      childRunId: "run-5700",
      finalMessage: "round-1 remediate",
      seq: 4,
    }),
  );

  const agents = (s.timeline ?? []).filter((it) => it.kind === "agent");
  assert.equal(agents.length, 2);
  assert.equal(agents[0].kind === "agent" && agents[0].finalMessage, "round-0 implement");
  assert.equal(agents[1].kind === "agent" && agents[1].finalMessage, "round-1 remediate");
});

test("replaying the same spawn event id does not duplicate the card", () => {
  let s = applyTimelineEvent(
    state(),
    event({ type: "agent_spawned_by_user", id: "spawn-1", agentName: "reviewer", childRunId: "run-6246", seq: 1 }),
  );
  s = applyTimelineEvent(
    { ...state(), ...s, timeline: s.timeline ?? [] },
    event({ type: "agent_spawned_by_user", id: "spawn-1", agentName: "reviewer", childRunId: "run-6246", seq: 1 }),
  );
  const agents = (s.timeline ?? []).filter((it) => it.kind === "agent");
  assert.equal(agents.length, 1);
  assert.equal(agents[0].kind === "agent" && agents[0].id, "spawn-1");
});

test("two distinct reviewers still render as spawn→result→spawn→result cards", () => {
  let s = applyTimelineEvent(
    state(),
    event({ type: "agent_spawned_by_user", id: "s1", agentName: "grok-review", childRunId: "run-6246", seq: 1 }),
  );
  s = applyTimelineEvent(
    { ...state(), ...s, timeline: s.timeline ?? [] },
    event({
      type: "agent_result_injected",
      id: "r1",
      agentName: "grok-review",
      childRunId: "run-6246",
      finalMessage: "changes requested A",
      seq: 2,
    }),
  );
  s = applyTimelineEvent(
    { ...state(), ...s, timeline: s.timeline ?? [] },
    event({ type: "agent_spawned_by_user", id: "s2", agentName: "my-reviewer", childRunId: "run-6254", seq: 3 }),
  );
  s = applyTimelineEvent(
    { ...state(), ...s, timeline: s.timeline ?? [] },
    event({
      type: "agent_result_injected",
      id: "r2",
      agentName: "my-reviewer",
      childRunId: "run-6254",
      finalMessage: "changes requested B",
      seq: 4,
    }),
  );

  assert.deepEqual(s.timeline, [
    { kind: "agent", id: "s1", agentName: "grok-review", childRunId: "run-6246", finalMessage: "changes requested A" },
    { kind: "agent", id: "s2", agentName: "my-reviewer", childRunId: "run-6254", finalMessage: "changes requested B" },
  ]);
});
