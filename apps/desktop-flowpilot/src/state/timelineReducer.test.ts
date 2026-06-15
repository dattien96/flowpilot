import test from "node:test";
import assert from "node:assert/strict";
import { applyTimelineEvent, shouldApplyRunEvent, type TimelineState, type TimelineItem } from "./timelineReducer";
import type { ProviderEventDTO } from "../types/contract";

function baseEvent(overrides: Partial<ProviderEventDTO>): ProviderEventDTO {
  return {
    id: "evt-1",
    workflowRunId: "run-1",
    providerSessionId: "thread-1",
    providerKey: "codex",
    seq: 1,
    occurredAt: "2026-06-13T10:00:00.000Z",
    type: "turn_completed",
    finalMessage: "done",
    ...overrides,
  } as ProviderEventDTO;
}

function thinkingState(timeline: TimelineItem[]): TimelineState {
  return {
    status: "running",
    timeline,
    recoverable: false,
    _streamingAssistantId: "assistant-1",
  };
}

test("tool_completed keeps the live thinking row at the end of the timeline", () => {
  const state = thinkingState([
    { kind: "prompt", id: "prompt-1", text: "Fix it" },
    { kind: "assistant", id: "assistant-1", text: "Looking...", finalized: false },
    { kind: "tool", id: "tool-1", toolName: "search", status: "running", input: { q: "bug" } },
    { kind: "thinking", id: "thinking-1", text: "Thinking..." },
  ]);

  const next = applyTimelineEvent(
    state,
    baseEvent({
      type: "tool_completed",
      toolName: "search",
      status: "success",
      output: { matches: 2 },
    }),
  );

  const nextTimeline = next.timeline ?? [];
  assert.equal(nextTimeline.at(-1)?.kind, "thinking");
  const tool = nextTimeline.find((item) => item.kind === "tool");
  assert.deepEqual(tool, {
    kind: "tool",
    id: "tool-1",
    toolName: "search",
    status: "success",
    input: { q: "bug" },
    output: { matches: 2 },
  });
});

test("turn_completed removes the thinking row once the answer is done", () => {
  const state = thinkingState([
    { kind: "prompt", id: "prompt-1", text: "Fix it" },
    { kind: "assistant", id: "assistant-1", text: "All set", finalized: true },
    { kind: "thinking", id: "thinking-1", text: "Thinking..." },
  ]);

  const next = applyTimelineEvent(
    state,
    baseEvent({
      type: "turn_completed",
      finalMessage: "All set",
    }),
  );

  assert.equal(next.timeline?.some((item) => item.kind === "thinking"), false);
});

test("run events only apply to the currently active run", () => {
  assert.equal(shouldApplyRunEvent("run-claude", "run-claude"), true);
  assert.equal(shouldApplyRunEvent("run-claude", "run-codex"), false);
  assert.equal(shouldApplyRunEvent(undefined, "run-codex"), false);
});
