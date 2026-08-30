import type { ProviderKey } from "@/types/contract";

// Providers that accept chat image attachments at the PROVIDER level
// (Task-052 / CA-483). codex + claude: native multimodal. grok: runner
// path-fallback writes <cwd>/.tmp/images and injects absolute paths into the
// text prompt.
//
// Task-318 locked opencode out of this set while Vision was false. Task-319
// proved the ACP image round-trip live (opencode 1.18.25: an image block sent
// to opencode/mimo-v2.5-free was seen by the model, turn completed), and
// live-verified that opencode image support is per-MODEL (models.dev
// input.image — 16/31 models). The set itself stays unchanged; opencode is
// unlocked per-model via supportsVisionFor below.
export const VISION_PROVIDERS = new Set<ProviderKey>([
  "codex",
  "claude",
  "grok",
]);

// supportsVisionFor is the SSOT image gate (Task-319): provider-level support
// from VISION_PROVIDERS, plus the per-model unlock for opencode — a selected
// opencode model whose detected `inputImage` (models.dev input.image) is true
// accepts image attachments; opencode models without the capability, unknown
// models, and every other provider without native support stay gated.
export function supportsVisionFor(
  provider: ProviderKey | null | undefined,
  modelInputImage?: boolean | null,
): boolean {
  if (provider && VISION_PROVIDERS.has(provider)) return true;
  return provider === "opencode" && modelInputImage === true;
}
