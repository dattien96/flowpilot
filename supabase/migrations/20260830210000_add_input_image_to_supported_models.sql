-- Task-319: capture per-model image-input capability on ai_supported_models.
--
-- `opencode models --verbose` (v1.18.25, live-verified 2026-08-30) reports
-- each model's models.dev capabilities.input.image. Opencode image support is
-- per-MODEL (16/31 models accept image input), so the desktop image-attach
-- gate consults this column instead of a blanket provider-level flag. Only
-- the runner/desktop populate it; existing rows stay null until re-detect.

alter table public.ai_supported_models
  add column if not exists input_image boolean null;
