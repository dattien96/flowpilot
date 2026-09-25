-- Migration: Persist the full flow-definition snapshot for user-owned flows (BUG-474).
--
-- step_definitions/workflows columns cannot express every execution-significant
-- FlowDefinition/FlowNode field (run, posture, context_profile, free-form
-- config, flow-level contextProfiles, tools). Built-in mirrors recover them
-- from the embedded pack; cloned/user-authored rows had no fallback and
-- silently dropped them on every save→reload round-trip — a cloned harness
-- flow could lose read_only posture or tournament config and execute with
-- silently different semantics.
--
-- definition_json stores the FlowDefinition snapshot the runner wrote at
-- save time for non-builtin rows. Real columns stay authoritative where both
-- exist (admin overrides win); the snapshot only fills fields the schema
-- cannot store. Built-in mirror rows leave it NULL — the embedded pack is
-- their sole authority.
alter table public.workflows
  add column if not exists definition_json jsonb;
