/**
 * Task-319: per-model image gate matrix. The provider-level set stays
 * locked (Task-318 guard untouched); opencode unlocks only when the selected
 * model carries the detected models.dev input.image capability.
 */
import test from "node:test";
import assert from "node:assert/strict";
import { supportsVisionFor, VISION_PROVIDERS } from "./visionProviders";

test("provider-level set stays locked: codex/claude/grok yes, opencode no", () => {
  assert.equal(VISION_PROVIDERS.has("codex"), true);
  assert.equal(VISION_PROVIDERS.has("claude"), true);
  assert.equal(VISION_PROVIDERS.has("grok"), true);
  assert.equal(VISION_PROVIDERS.has("opencode"), false);
});

test("opencode unlocks only with a model whose inputImage is true", () => {
  assert.equal(supportsVisionFor("opencode", true), true);
  assert.equal(supportsVisionFor("opencode", false), false);
  assert.equal(supportsVisionFor("opencode", null), false);
  assert.equal(supportsVisionFor("opencode", undefined), false);
});

test("other providers never unlock via the model flag", () => {
  assert.equal(supportsVisionFor("codex", null), true);
  assert.equal(supportsVisionFor("gemini", true), false);
  assert.equal(supportsVisionFor("claude", false), true);
  assert.equal(supportsVisionFor(null, true), false);
  assert.equal(supportsVisionFor(undefined, undefined), false);
});
