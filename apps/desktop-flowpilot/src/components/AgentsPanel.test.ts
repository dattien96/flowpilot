import test from "node:test";
import assert from "node:assert/strict";
import { formatDependencyLabels } from "./agentDependencies";
import { agentRunDisplayName, resolveMainAgentDisplay } from "./AgentsPanel";
import type { AgentRunSummary } from "@/types/contract";

const runs: AgentRunSummary[] = [
  {
    runId: "run-reviewer",
    agentName: "reviewer",
    role: "reviewer",
    status: "completed",
    createdAt: "2026-06-20T00:00:00Z",
  },
  {
    runId: "run-tester",
    agentName: "tester",
    role: "tester",
    status: "starting",
    createdAt: "2026-06-20T00:00:01Z",
  },
];

test("formatDependencyLabels resolves dependency run ids to agent names", () => {
  assert.deepEqual(formatDependencyLabels(["run-reviewer", "missing-run"], runs), ["reviewer", "missing-run"]);
});

test("agentRunDisplayName prefers flow node label over generic agent name", () => {
  assert.equal(agentRunDisplayName({ agentName: "reviewer-agent", label: "review-security-gpt" }), "review-security-gpt");
  assert.equal(agentRunDisplayName({ agentName: "reviewer-agent" }), "reviewer-agent");
});

// BUG-227: a started run's actual resolved posture (workflowStepRuntimeMeta,
// e.g. Claude Haiku from the workflow's model_override) must win on the main
// card even when the pre-run catalog preview (selectedWorkflow?.model ||
// project?.model) resolves to a different provider/model, such as a
// project-level Codex default masking the run's real Claude posture.
test("resolveMainAgentDisplay: runtime meta wins over the pre-run catalog preview", () => {
  const { mainProvider, mainModel } = resolveMainAgentDisplay({
    resolvedProvider: "codex",
    resolvedModel: "gpt-5.4-mini",
    runtimeMetaProvider: "claude",
    runtimeMetaModel: "claude-haiku",
    selectedProvider: "codex",
    selectedModel: "gpt-5.4-mini",
  });
  assert.equal(mainProvider, "claude");
  assert.equal(mainModel, "claude-haiku");
});

test("resolveMainAgentDisplay: falls back to the pre-run catalog preview before any run has started", () => {
  const { mainProvider, mainModel } = resolveMainAgentDisplay({
    resolvedProvider: "claude",
    resolvedModel: "claude-haiku",
    runtimeMetaProvider: undefined,
    runtimeMetaModel: undefined,
    selectedProvider: "codex",
    selectedModel: "gpt-5.4-mini",
  });
  assert.equal(mainProvider, "claude");
  assert.equal(mainModel, "claude-haiku");
});

test("resolveMainAgentDisplay: falls back to the last chat selection, then codex, when nothing else resolves", () => {
  const withSelection = resolveMainAgentDisplay({
    resolvedProvider: "",
    resolvedModel: "",
    runtimeMetaProvider: undefined,
    runtimeMetaModel: undefined,
    selectedProvider: "codex",
    selectedModel: "gpt-5.4-mini",
  });
  assert.equal(withSelection.mainProvider, "codex");
  assert.equal(withSelection.mainModel, "gpt-5.4-mini");

  const withNothing = resolveMainAgentDisplay({
    resolvedProvider: "",
    resolvedModel: "",
    runtimeMetaProvider: undefined,
    runtimeMetaModel: undefined,
    selectedProvider: "",
    selectedModel: "",
  });
  assert.equal(withNothing.mainProvider, "codex");
  assert.equal(withNothing.mainModel, "");
});
