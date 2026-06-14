import test from "node:test";
import assert from "node:assert/strict";

import {
  assertValidProjectBindings,
  resolveProviderKeyForModel,
} from "../../packages/flowpilot-client-core/src/domain/adminLogic";

test("resolveProviderKeyForModel handles codex, gemini, auto-gemini, and claude prefixes", () => {
  assert.equal(resolveProviderKeyForModel("gpt-5.4", []), "codex");
  assert.equal(resolveProviderKeyForModel("gemini-2.5-pro", []), "gemini");
  assert.equal(resolveProviderKeyForModel("auto-gemini-2.5-pro", []), "gemini");
  assert.equal(resolveProviderKeyForModel("claude-sonnet-4", []), "claude");
});

test("resolveProviderKeyForModel falls back to supported model registry", () => {
  assert.equal(
    resolveProviderKeyForModel("custom-model", [
      {
        id: "model-1",
        providerKey: "gemini",
        modelId: "custom-model",
        displayName: "Custom Model",
        isEnabled: true,
        sortOrder: 1,
        source: "manual",
        detectionMethod: null,
        detectedCliVersion: null,
        lastDetectedAt: null,
        createdAt: "",
        updatedAt: "",
      },
    ]),
    "gemini",
  );
});

test("assertValidProjectBindings rejects empty and duplicate paths", () => {
  assert.throws(() => assertValidProjectBindings([]), /Add at least one directory binding/);
  assert.throws(
    () =>
      assertValidProjectBindings([
        { localPath: "/workspace/app" },
        { localPath: "/workspace/app" },
      ]),
    /Directory bindings must use unique paths/,
  );
});

test("assertValidProjectBindings accepts unique trimmed paths", () => {
  assert.doesNotThrow(() =>
    assertValidProjectBindings([
      { localPath: " /workspace/app " },
      { localPath: "/workspace/api" },
    ]),
  );
});
