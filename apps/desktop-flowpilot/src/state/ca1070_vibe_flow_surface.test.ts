import "./localStorageTestStub";
import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import {
  filterWorkflowsForWorkingMode,
  userFlowSelectableForMode,
} from "./workingMode";

// CA-1070: the Flow tab's workflow select must mirror the runner's
// workingmode.FlowAllowedForWorkingMode user-start gate — dev sees harness +
// catalog flows, vibe sees only the user-startable vibe flows (CP-90 adds
// vibe-tasks as the third).
const WORKFLOWS = [
  { id: "wf-task", projectId: "", name: "Task Harness", packFlowId: "task-harness" },
  { id: "wf-ingest", projectId: "", name: "Vibe Ingest", packFlowId: "vibe-ingest" },
  { id: "wf-cp", projectId: "", name: "Vibe CP Ingest", packFlowId: "vibe-cp-ingest" },
  { id: "wf-tasks", projectId: "", name: "Vibe Tasks", packFlowId: "vibe-tasks" },
  { id: "wf-sprint", projectId: "", name: "Vibe Sprint", packFlowId: "vibe-sprint" },
  { id: "wf-review", projectId: "", name: "Review Loop", packFlowId: "review-loop" },
  { id: "user-uuid-1", projectId: "", name: "My Flow" },
];

test("dev flow list hides vibe-family and non-startable builtin mirrors", () => {
  const ids = filterWorkflowsForWorkingMode(WORKFLOWS, "dev").map((w) => w.id);
  assert.deepEqual(ids, ["wf-task", "user-uuid-1"]);
});

test("vibe flow list shows only the user-startable vibe flows", () => {
  const ids = filterWorkflowsForWorkingMode(WORKFLOWS, "vibe")
    .map((w) => w.id)
    .sort();
  assert.deepEqual(ids, ["wf-cp", "wf-ingest", "wf-tasks"]);
});

test("userFlowSelectableForMode mirrors the runner family gate", () => {
  // dev: harness five + untracked catalog ids pass; vibe-family and hidden
  // (never-startable) mirrors are rejected.
  assert.ok(userFlowSelectableForMode("dev", "task-harness"));
  assert.ok(userFlowSelectableForMode("dev", "user-uuid-1"));
  assert.ok(userFlowSelectableForMode("dev", "flowpilot-core-flow-pack/task-harness"));
  assert.ok(!userFlowSelectableForMode("dev", "vibe-ingest"));
  assert.ok(!userFlowSelectableForMode("dev", "vibe-sprint"));
  assert.ok(!userFlowSelectableForMode("dev", "review-loop"));
  assert.ok(!userFlowSelectableForMode("dev", ""));
  // vibe: only the vibe user set — system vibe flows and everything else out.
  assert.ok(userFlowSelectableForMode("vibe", "vibe-ingest"));
  assert.ok(userFlowSelectableForMode("vibe", "vibe-cp-ingest"));
  assert.ok(userFlowSelectableForMode("vibe", "vibe-tasks"));
  assert.ok(!userFlowSelectableForMode("vibe", "vibe-sprint"));
  assert.ok(!userFlowSelectableForMode("vibe", "task-harness"));
  assert.ok(!userFlowSelectableForMode("vibe", "user-uuid-1"));
});

test("vibe on from chat mode auto-switches to the flow tab", () => {
  useStore.setState({ workingMode: "dev", chatMode: "normal_chat", runId: undefined });
  useStore.getState().setWorkingMode("vibe");
  const s = useStore.getState();
  assert.equal(s.workingMode, "vibe");
  assert.equal(s.chatMode, "workflow_step_auto");
});

test("vibe on while already on the flow tab stays on it", () => {
  useStore.setState({ workingMode: "dev", chatMode: "workflow_step_auto", runId: undefined });
  useStore.getState().setWorkingMode("vibe");
  assert.equal(useStore.getState().chatMode, "workflow_step_auto");
});

test("chat tab cannot be re-entered while vibe is on", () => {
  useStore.setState({ workingMode: "vibe", chatMode: "workflow_step_auto", runId: undefined });
  useStore.getState().setChatMode("normal_chat");
  assert.equal(useStore.getState().chatMode, "workflow_step_auto");
  // Flow tab itself is still switchable under vibe.
  useStore.getState().setChatMode("workflow_step_auto");
  assert.equal(useStore.getState().chatMode, "workflow_step_auto");
});

test("vibe off keeps the flow tab and re-enables the chat tab", () => {
  useStore.setState({ workingMode: "vibe", chatMode: "workflow_step_auto", runId: undefined });
  useStore.getState().setWorkingMode("dev");
  assert.equal(useStore.getState().chatMode, "workflow_step_auto");
  useStore.getState().setChatMode("normal_chat");
  assert.equal(useStore.getState().chatMode, "normal_chat");
});

test("mode flip clears a workflow selection the new mode cannot start", () => {
  useStore.setState({
    workingMode: "dev",
    chatMode: "workflow_step_auto",
    runId: undefined,
    workflows: WORKFLOWS as never,
    selectedWorkflowId: "wf-task",
  });
  useStore.getState().setWorkingMode("vibe");
  assert.equal(useStore.getState().selectedWorkflowId, undefined);
  // Flipping back to dev drops a vibe-only pick too.
  useStore.setState({ selectedWorkflowId: "wf-ingest" });
  useStore.getState().setWorkingMode("dev");
  assert.equal(useStore.getState().selectedWorkflowId, undefined);
  // An allowed selection survives the flip.
  useStore.setState({ workingMode: "vibe", chatMode: "workflow_step_auto", selectedWorkflowId: "wf-ingest" });
  useStore.getState().setWorkingMode("vibe");
  assert.equal(useStore.getState().selectedWorkflowId, "wf-ingest");
});
