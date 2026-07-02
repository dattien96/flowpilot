-- BUG-xxx: step_definitions.model must stay in sync with the dynamic
-- ai_supported_models registry (Task-013). The registry lets admins add a
-- model at any time via Settings > AI Providers, and workflow-steps forms
-- immediately offer that model in their dropdown (useSupportedModels()).
-- Saving a step with such a model previously violated this rigid CHECK
-- constraint because the constraint's allow-list is frozen at migration
-- time and has no way to learn about newly registered models.
--
-- The application already validates/derives models against
-- ai_supported_models (see normalizeStepModel / useSupportedModels), so the
-- database no longer needs a hard-coded allow-list here. This implements
-- Migration C from Task-013.
alter table public.step_definitions
  drop constraint if exists step_definitions_supported_model_check;
