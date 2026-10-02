import test from "node:test";
import assert from "node:assert/strict";
import { scrollAnchorCandidates, scrollAnchorRevealPrompts } from "./timelineScrollAnchor";
import type { TimelineItem } from "@/state/store";

const approvalDetails = { decisions: [{ value: "approve", label: "Approve" }] };

const timeline: TimelineItem[] = [
  { kind: "prompt", id: "p1", text: "p1" },
  { kind: "assistant", id: "a1", text: "a1", finalized: true },
  { kind: "prompt", id: "p2", text: "p2" },
  { kind: "assistant", id: "a2", text: "a2", finalized: true },
  { kind: "approval", id: "ap-1", approvalId: "ap-1", details: approvalDetails },
  { kind: "prompt", id: "p3", text: "p3" },
  { kind: "assistant", id: "a3", text: "a3", finalized: true },
];

test("scrollAnchorCandidates covers the regrouped approval/question forms", () => {
  assert.deepEqual(scrollAnchorCandidates("approval-3"), [
    "approval-3",
    "approval-group-approval-3",
    "question-group-approval-3",
  ]);
});

test("scrollAnchorRevealPrompts returns 1 for an item in the last prompt window", () => {
  assert.equal(scrollAnchorRevealPrompts(timeline, "a3"), 1);
});

test("scrollAnchorRevealPrompts returns the prompt count to reveal an older item", () => {
  assert.equal(scrollAnchorRevealPrompts(timeline, "a2"), 2);
  assert.equal(scrollAnchorRevealPrompts(timeline, "p1"), 3);
});

test("scrollAnchorRevealPrompts resolves a grouped id back to its raw item", () => {
  assert.equal(scrollAnchorRevealPrompts(timeline, "approval-group-ap-1"), 2);
});

test("scrollAnchorRevealPrompts returns undefined when the anchored id vanished", () => {
  assert.equal(scrollAnchorRevealPrompts(timeline, "approval-9"), undefined);
  assert.equal(scrollAnchorRevealPrompts(timeline, "tool-group-tool-9"), undefined);
});
