# Task-319: Opencode Per-Model Image Capability — Unlock Paste/Attach

## Metadata

- Document ID: `Task-319`
- Title: `Opencode Per-Model Image Capability — Unlock Paste/Attach`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-30`
- Last Updated: `2026-08-30`
- Parent Documents: [CP-57: Opencode Provider Integration](../../07-Coding-Plan/done/CP-57-Opencode-Provider-Integration.md)
- Child Documents: `None`
- Related Documents: [Task-318: Opencode Vision=false Guard](../done/Task-318-Opencode-Vision-False-Guard.md), [Task-215: Per-Model Reasoning-Effort Detection](../done/Task-215-Per-Model-Reasoning-Effort-Detection-And-Model-Aware-UI.md), CA-693
- Replaces: `None`
- Tags: `opencode, vision, per-model-capability, ai-providers, image-attachments`

## AI Quick View

### Summary

- Opencode image support is **per-MODEL, not per-provider**: `opencode models --verbose` (v1.18.25, live 2026-08-30) reports models.dev `capabilities.input.image` — 16/31 `opencode/*`+`opencode-go/*` models accept image input, 15 do not.
- The Task-318 provider-level guard blocked ALL opencode models. This task unlocks image paste/attach only for models whose detected `inputImage` is true, keeping non-vision models gated.
- The Task-318 unlock condition is **MET**: live ACP probe (scripted JSON-RPC over `opencode acp` stdio) sent `{type:"image", data:<base64>, mimeType:"image/png"}` to `opencode/mimo-v2.5-free` — the model described the image ("Red") and the turn completed (`stopReason: end_turn`). No `.tmp/images` path fallback needed; native ACP image blocks work.

### Key Decisions

- `T-1` **Per-model gate SSOT** — `supportsVisionFor(provider, modelInputImage)` (desktop) / `chatSupportsImages()` (TUI) / `opencodeModelSupportsImages()` (runner adapter): provider-level `VISION_PROVIDERS`/`SupportsImages` stays codex/claude/grok; opencode unlocks only when the selected model's `inputImage` is true. Unknown/absent model → conservative false.
- `T-2` **Detection via `--verbose` probe** (Task-215 pattern) — `detectOpencodeModelsLive` now spawns `opencode models --verbose`; the parser captures `capabilities.input.image` into `ProviderModel.InputImage`. Plain-line catalogs (older CLIs) still parse via fallback with `InputImage=false`; CLI versions that reject `--verbose` fall back to the plain `models` spawn (CA-657 timeout budget unchanged).
- `T-3` **Native ACP image blocks, not path fallback** — `opencodeACPPromptParamsWithAttachments` appends `{type:"image", data:<base64>, mimeType}` blocks (live-proven shape). Grok CA-483 `.tmp/images` fallback is NOT needed for opencode.
- `T-4` **Provider-level `Vision:false` stays** — same pattern as Grok (`grok_adapter.go:225`): the capability flag is provider-wide, the per-model catalog is the truth; UI gates and the adapter consult it directly.
- `T-5` **Runner defense-in-depth** — `opencodeTurnImageAttachments` drops attachments unless the selected model is image-capable, so a stale UI can never blind-copy bytes to a model that cannot see them.

### Constraints

- Task-318's additive-tests-only contract honored: `TestOpencodeCapabilitiesMatchProvenSet`, `TestOpencodeACPPromptParamsBuildsCorrectJSON`, and `TestOpencodePromptParamsNeverBuildsImageBlock` stay untouched and green (the prompt-params guard tests the no-attachments shape, which is unchanged).
- `feature_key: ai-providers`.

## 6. Acceptance Check

- Desktop: paste/attach enabled for opencode only when the selected model has detected `inputImage=true` (requires one "Detect models" run after upgrade to populate the catalog column).
- TUI: `/image <path>`, clipboard paste, and image-path paste follow the same per-model gate; the error copy for non-vision opencode models names the model gate ("models.dev input.image=false — pick a vision model").
- Runner: image-capable model turns emit text + image blocks; non-capable/unknown models ignore attachments.

### 6.1 Definition of Done (DOD)

- [x] `DOD-1` Runner detection captures `InputImage` from `opencode models --verbose` (+ plain/old-CLI fallbacks); fixture tests.
- [x] `DOD-2` Types plumbed end-to-end: runner `ProviderModel` → GET /providers → TUI `client.ProviderModel` → client-core `LocalRunnerProviderModel`/`SupportedModel` → desktop; Supabase migration `20260830210000_add_input_image_to_supported_models.sql`.
- [x] `DOD-3` Desktop + TUI gates per-model (`supportsVisionFor` / `chatSupportsImages`); AiProvidersSettings detect stamps `inputImage`.
- [x] `DOD-4` Runner adapter sends ACP image blocks for capable models; defense-in-depth gate; Task-318 guard tests untouched & green.
- [x] `DOD-5` Live round-trip proof recorded (mimo-v2.5-free, "Red", end_turn) — Task-318 unlock condition satisfied.

## 8. Completion Notes

- result: closed. All gates and plumbing implemented; 30+ new/updated tests green; desktop typecheck at baseline (pre-existing errors only).
- follow-ups:
  - Gemini ACP adapter still has no image path (separate task if ever needed).
  - `limit.context` from the verbose blob could populate `ContextWindowTokens` for opencode (Task-215 field) — intentionally not done here to keep the change scoped.
