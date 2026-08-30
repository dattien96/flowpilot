/**
 * Task-318 (CP-57 P-12 / Task-303 T-7 / DOD-15 / CP-57-Test-Steps M1):
 * Opencode Vision stays false until the ACP image attachment round-trip is
 * proven. The desktop gate is `VISION_PROVIDERS` — it must exclude `opencode`
 * so the attach button is disabled and paste is blocked ("chặn trước khi gửi
 * prompt").
 *
 * additive-tests-only: new file only.
 * cross-provider-parity: matrix over codex/claude/grok (the proven set).
 */
import test from "node:test";
import assert from "node:assert/strict";
import { VISION_PROVIDERS } from "./visionProviders";

test("VISION_PROVIDERS includes the proven multimodal providers", () => {
  assert.equal(VISION_PROVIDERS.has("codex"), true);
  assert.equal(VISION_PROVIDERS.has("claude"), true);
  assert.equal(VISION_PROVIDERS.has("grok"), true);
});

test("VISION_PROVIDERS excludes opencode until the ACP image round-trip is proven", () => {
  // CP-57 P-12 / Task-303 T-7 / DOD-15: Vision stays false until proven.
  // opencode_acp.go:88-90 records promptCapabilities.image was live-verified
  // true in the initialize handshake, but the attachment round-trip (Task-301)
  // was never proven — so the desktop gate must keep opencode out.
  assert.equal(VISION_PROVIDERS.has("opencode"), false);
});
