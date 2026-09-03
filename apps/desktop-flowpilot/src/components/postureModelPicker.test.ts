/**
 * BUG-348: posture setup modal model picker — no provider select, provider
 * auto-inferred from the picked model (mirrors TUI mode_setup_modal.go).
 */
import test from "node:test";
import assert from "node:assert/strict";
import type { LocalRunnerProvider, SupportedModel } from "@flowpilot/client-core";
import {
  applyPostureModelPick,
  buildPostureModelGroups,
  inferPostureProvider,
  postureReasoningOptions,
} from "./postureModelPicker";

function supportedModel(over: Partial<SupportedModel> & { providerKey: SupportedModel["providerKey"]; modelId: string }): SupportedModel {
  return {
    id: `${over.providerKey}/${over.modelId}`,
    displayName: over.modelId,
    isEnabled: true,
    sortOrder: 0,
    source: "test",
    detectionMethod: null,
    detectedCliVersion: null,
    lastDetectedAt: null,
    supportedReasoningEfforts: null,
    defaultReasoningEffort: null,
    contextWindowTokens: null,
    maxContextWindowTokens: null,
    inputImage: null,
    createdAt: "",
    updatedAt: "",
    ...over,
  };
}

const supported: SupportedModel[] = [
  supportedModel({ providerKey: "claude", modelId: "opus", displayName: "Opus" }),
  supportedModel({ providerKey: "claude", modelId: "sonnet", isEnabled: false }),
  supportedModel({
    providerKey: "grok",
    modelId: "grok-4.5",
    supportedReasoningEfforts: ["Low", "HIGH"],
  }),
];

const detected: LocalRunnerProvider[] = [
  {
    key: "codex",
    label: "Codex",
    installed: true,
    version: null,
    installHint: null,
    models: [{ id: "o3", displayName: "o3", available: true, source: "test" }],
  },
];

test("inferPostureProvider finds the owning provider (case-insensitive)", () => {
  assert.equal(inferPostureProvider(supported, detected, "opus"), "claude");
  assert.equal(inferPostureProvider(supported, detected, "GROK-4.5"), "grok");
  assert.equal(inferPostureProvider(supported, detected, "o3"), "codex");
});

test("inferPostureProvider returns undefined for unknown/empty models", () => {
  assert.equal(inferPostureProvider(supported, detected, "nope"), undefined);
  assert.equal(inferPostureProvider(supported, detected, ""), undefined);
  assert.equal(inferPostureProvider(supported, detected, "  "), undefined);
});

test("applyPostureModelPick sets model + auto-infers provider", () => {
  assert.deepEqual(applyPostureModelPick(supported, detected, "grok-4.5"), {
    modelId: "grok-4.5",
    provider: "grok",
    reasoningEffort: undefined,
  });
});

test("applyPostureModelPick clears provider for unknown models (TUI parity)", () => {
  assert.deepEqual(applyPostureModelPick(supported, detected, "mystery"), {
    modelId: "mystery",
    provider: undefined,
    reasoningEffort: undefined,
  });
});

test("applyPostureModelPick on inherit clears model + provider + reasoning", () => {
  assert.deepEqual(applyPostureModelPick(supported, detected, ""), {
    modelId: "",
    provider: undefined,
    reasoningEffort: undefined,
  });
});

test("buildPostureModelGroups lists enabled registry models grouped by provider", () => {
  const groups = buildPostureModelGroups(supported, detected);
  const keys = groups.map((g) => g.providerKey);
  assert.deepEqual(keys, ["claude", "grok", "codex"]);
  assert.deepEqual(
    groups.find((g) => g.providerKey === "claude")!.options.map((o) => o.modelId),
    ["opus"],
  );
});

test("buildPostureModelGroups keeps the current pin even when absent from lists", () => {
  const groups = buildPostureModelGroups(supported, [], "custom-model", "gemini");
  const all = groups.flatMap((g) => g.options.map((o) => o.modelId));
  assert.equal(all.includes("custom-model"), true);
  assert.equal(groups.find((g) => g.options.some((o) => o.modelId === "custom-model"))!.providerKey, "gemini");
});

test("postureReasoningOptions is null while no model is picked (disabled)", () => {
  assert.equal(postureReasoningOptions(supported, ""), null);
});

test("postureReasoningOptions uses the model catalog efforts when known", () => {
  assert.deepEqual(postureReasoningOptions(supported, "grok-4.5"), ["low", "high"]);
});

test("postureReasoningOptions falls back to the full default list", () => {
  const opts = postureReasoningOptions(supported, "opus");
  assert.ok(opts && opts.includes("low") && opts.includes("max") && opts.includes("minimal"));
});
