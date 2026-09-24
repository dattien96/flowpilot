import test from "node:test";
import assert from "node:assert/strict";
import type { SupportedModel } from "@flowpilot/client-core";
import { staleDetectedModelRows } from "./aiProvidersDetect";

// Task-438: after a live detect, previously-detected rows that vanished from
// the provider catalog must be disabled (not deleted), while manually-added
// and other-provider rows stay untouched.

function fakeModel(overrides: Partial<SupportedModel> = {}): SupportedModel {
  return {
    id: overrides.id ?? "row-1",
    providerKey: overrides.providerKey ?? "devin",
    modelId: overrides.modelId ?? "swe-2-high",
    displayName: overrides.displayName ?? "SWE-2",
    isEnabled: overrides.isEnabled ?? true,
    sortOrder: overrides.sortOrder ?? 1,
    source: overrides.source ?? "detected",
    detectionMethod: overrides.detectionMethod ?? "devin-acp",
    detectedCliVersion: null,
    lastDetectedAt: null,
    supportedReasoningEfforts: null,
    defaultReasoningEffort: null,
    contextWindowTokens: null,
    maxContextWindowTokens: null,
    inputImage: null,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  };
}

test("staleDetectedModelRows flags detected rows absent from the live catalog", () => {
  const registered = [
    fakeModel({ id: "row-stale", modelId: "swe-2-max" }),
    fakeModel({ id: "row-live", modelId: "swe-2-high" }),
  ];
  const detected = [{ id: "swe-2-high" }, { id: "claude-opus-4-7-medium" }];
  const stale = staleDetectedModelRows(registered, detected, "devin");
  assert.deepEqual(
    stale.map((r) => r.id),
    ["row-stale"],
  );
});

test("staleDetectedModelRows never flags manually-added models", () => {
  const registered = [
    fakeModel({ id: "row-manual", modelId: "my-custom-devin", source: "manual" }),
    fakeModel({ id: "row-detected", modelId: "swe-2-max", source: "detected" }),
  ];
  const detected = [{ id: "swe-2-high" }];
  const stale = staleDetectedModelRows(registered, detected, "devin");
  assert.deepEqual(
    stale.map((r) => r.id),
    ["row-detected"],
  );
});

test("staleDetectedModelRows skips already-disabled rows and other providers", () => {
  const registered = [
    fakeModel({ id: "row-off", modelId: "swe-2-max", isEnabled: false }),
    fakeModel({ id: "row-codex", modelId: "gpt-5-stale", providerKey: "codex" }),
    fakeModel({ id: "row-stale", modelId: "swe-2-medium" }),
  ];
  const detected = [{ id: "swe-2-high" }];
  const stale = staleDetectedModelRows(registered, detected, "devin");
  assert.deepEqual(
    stale.map((r) => r.id),
    ["row-stale"],
  );
});

test("staleDetectedModelRows returns empty when every detected row is still live", () => {
  const registered = [fakeModel({ id: "row-live", modelId: "swe-2-high" })];
  const detected = [{ id: "swe-2-high" }];
  assert.equal(staleDetectedModelRows(registered, detected, "devin").length, 0);
});
