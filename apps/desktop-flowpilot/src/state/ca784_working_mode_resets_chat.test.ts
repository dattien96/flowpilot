import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";

test("vibe on from bug intent returns to normal chat", () => {
  useStore.setState({
    workingMode: "dev",
    chatStartMode: "bugfix",
    chatSourceDocId: "BUG-1",
    flowRef: "task-harness",
    builtinOrchestrationOptions: [{ flowRef: "task-harness", label: "Task Harness", description: "" }],
  });
  useStore.getState().setWorkingMode("vibe");
  const s = useStore.getState();
  assert.equal(s.workingMode, "vibe");
  assert.equal(s.chatStartMode, "normal");
  assert.equal(s.chatSourceDocId, "");
  assert.equal(s.flowRef, undefined);
  assert.deepEqual(s.builtinOrchestrationOptions, []);
});

test("vibe off from vibe path returns to normal chat", () => {
  useStore.setState({
    workingMode: "vibe",
    chatStartMode: "normal",
    chatSourceDocId: "requirements/07-Coding-Plan/inprogress/CP-60-Vibe-Working-Mode.md",
    flowRef: "vibe-ingest",
  });
  useStore.getState().setWorkingMode("dev");
  const s = useStore.getState();
  assert.equal(s.workingMode, "dev");
  assert.equal(s.chatStartMode, "normal");
  assert.equal(s.chatSourceDocId, "");
  assert.equal(s.flowRef, undefined);
});
