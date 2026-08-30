import type { ProviderKey } from "@/types/contract";

// Providers that accept chat image attachments (Task-052 / CA-483).
// codex + claude: native multimodal. grok: runner path-fallback writes
// <cwd>/.tmp/images and injects absolute paths into the text prompt.
//
// Task-318 (CP-57 P-12 / Task-303 T-7 / DOD-15): opencode is intentionally
// EXCLUDED — Vision stays false until the ACP image attachment round-trip is
// proven. opencode_acp.go:88-90 records promptCapabilities.image was
// live-verified true in the initialize handshake, but the attachment
// round-trip (Task-301) was never proven. Do not add opencode here without
// that proof.
export const VISION_PROVIDERS = new Set<ProviderKey>([
  "codex",
  "claude",
  "grok",
]);
