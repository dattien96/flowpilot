-- Task-259 follow-up / CP-43 P-6 + CP-55 P-8: bring the built-in
-- context_artifact instance in parity with runner defaultContextSourceIDs.
-- The original 20260709092000 seed had only the 3 CP-44 sources
-- (feature.history, chat.summary, source.excerpt). CP-50 (canonical.head)
-- and CP-55 (change.contract) and Task-259 (source.dependence) joined the
-- default set later, but the artifact was never updated, so CCRS's artifact
-- binding (highest precedence tier) was cutting those three sources.
-- This migration makes the artifact reproduce the full 6-id default set.
update artifact_instances
set config_json = '{"sources": ["canonical.head", "feature.history", "change.contract", "source.dependence", "chat.summary", "source.excerpt"]}'::jsonb,
    name = 'Default Context (All Sources)',
    description = 'Built-in context artifact using the full default source set (canonical head, feature history, change contract, source dependence, chat summary, source excerpts).',
    updated_at = now()
where id = '00000000-0000-0000-0000-000000000001';
