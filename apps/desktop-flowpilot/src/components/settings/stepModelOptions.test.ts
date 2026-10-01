import test from "node:test";
import assert from "node:assert/strict";
import {
  buildModelOptions,
  filterModelOptions,
  groupModelOptions,
  modelProviderLabel,
} from "./stepModelOptions";

const catalog = [
  { modelId: "claude-sonnet", displayName: "Claude Sonnet", providerKey: "claude", isEnabled: true },
  { modelId: "gpt-5.4", displayName: "GPT 5.4", providerKey: "codex", isEnabled: true },
  { modelId: "gpt-5.4-mini", displayName: "GPT 5.4 Mini", providerKey: "codex", isEnabled: true },
  { modelId: "devin/swe-2-high", displayName: "Devin SWE-2 (High)", providerKey: "devin", isEnabled: true },
  { modelId: "devin/swe-2-low", displayName: "Devin SWE-2 (Low)", providerKey: "devin", isEnabled: false },
  { modelId: "grok-4.6", displayName: "Grok 4.6", providerKey: "grok", isEnabled: true },
];

test("buildModelOptions: only enabled rows, carrying providerKey", () => {
  const options = buildModelOptions(catalog);
  assert.equal(options.length, 5);
  assert.equal(options.find((o) => o.value === "devin/swe-2-low"), undefined);
  assert.equal(options[3].providerKey, "devin");
});

test("groupModelOptions: folds by provider preserving order, swe-2 reachable under Devin", () => {
  const groups = groupModelOptions(buildModelOptions(catalog));
  assert.deepEqual(groups.map((g) => g.providerKey), ["claude", "codex", "devin", "grok"]);
  assert.deepEqual(groups.map((g) => g.providerLabel), ["Claude", "Codex", "Devin", "Grok"]);
  const devin = groups.find((g) => g.providerKey === "devin")!;
  assert.deepEqual(devin.options.map((o) => o.value), ["devin/swe-2-high"]);
});

test("filterModelOptions: matches display name, model id, and provider", () => {
  const options = buildModelOptions(catalog);
  assert.deepEqual(filterModelOptions(options, "swe").map((o) => o.value), ["devin/swe-2-high"]);
  assert.deepEqual(filterModelOptions(options, "DEVIN").map((o) => o.value), ["devin/swe-2-high"]);
  assert.deepEqual(filterModelOptions(options, "gpt").map((o) => o.value), ["gpt-5.4", "gpt-5.4-mini"]);
  assert.equal(filterModelOptions(options, "nonexistent").length, 0);
  assert.equal(filterModelOptions(options, "  ").length, 5);
});

test("modelProviderLabel: known keys map, unknown falls back capitalized", () => {
  assert.equal(modelProviderLabel("devin"), "Devin");
  assert.equal(modelProviderLabel("futuristic-llm"), "Futuristic-llm");
  assert.equal(modelProviderLabel(""), "Other");
});
