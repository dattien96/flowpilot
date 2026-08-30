# CA-693 — Opencode per-model image capability unlock (Task-319)

## Problem

The Task-318 guard excluded `opencode` from image support at the PROVIDER level
(`VISION_PROVIDERS`, `SupportsImages`, `Capabilities.Vision=false`). Live data
(`opencode models --verbose`, v1.18.25, 2026-08-30) proves image support is
per-MODEL: 16/31 `opencode/*`+`opencode-go/*` models report models.dev
`capabilities.input.image=true` (mimo-v2.5, kimi-k2.6/2.7/3, glm-5.3-flash,
gpt-5.6-luna, grok-4.6, qwen3.6-plus/3.7-plus/3.8-*, …), 15 report false. The
blanket provider gate therefore blocked paste/attach for models that CAN see
images ("provider opencode does not support image attachments").

## Change

- **Runner detection** — `detectOpencodeModelsLive` probes `opencode models
  --verbose` and parses each model's JSON blob
  (`capabilities.input.image` → new `ProviderModel.InputImage`). Plain-line
  catalogs and CLIs rejecting `--verbose` fall back to the previous plain
  `models` spawn (CA-657 timeout budget unchanged).
- **Plumbing** — `input_image` carried through GET /providers, TUI
  `client.ProviderModel`, client-core `LocalRunnerProviderModel`/
  `SupportedModel` (+ Supabase migration `20260830210000`), desktop
  `AiProvidersSettings` detect sync.
- **Gates** — desktop `supportsVisionFor(provider, modelInputImage)`
  (`visionProviders.ts`), TUI `chatSupportsImages()`/`imagesUnsupportedReason()`
  (`tui/app/image_model_gate.go`) wired into `/image`, clipboard paste, and
  image-path paste. `ValidateAttachmentsForModel` in `tui/client/image.go`.
- **Adapter** — `opencodeACPPromptParamsWithAttachments` appends ACP image
  blocks `{type:"image", data:<base64>, mimeType}` (text block stays first);
  `opencodeTurnImageAttachments` gates on the selected model's capability
  (defense-in-depth). Provider-level `Vision:false` retained (Grok pattern).

## Live proof (Task-318 unlock condition MET)

Scripted JSON-RPC over `opencode acp` stdio (2026-08-30, 1.18.25):
initialize → session/new → `session/set_config_option {configId:"model",
value:"opencode/mimo-v2.5-free"}` → `session/prompt` with text + image block
(2×2 red PNG, base64). Result: model answered **"Red"** (saw the image),
`stopReason: end_turn`. No `.tmp/images` path fallback needed for opencode.

# ---8<--- flowpilot:change-ledger

feature_key: ai-providers
source_doc_id: Task-319
change_type: feature
summary: Unlock opencode image paste/attach per model — verbose models probe captures models.dev input.image; desktop/TUI gates per-model; runner sends ACP image blocks for capable models

# --->8---
