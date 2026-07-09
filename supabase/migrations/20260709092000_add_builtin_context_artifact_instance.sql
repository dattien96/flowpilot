-- CP-45 Task-201: built-in default `context_artifact.v1` instance.
--
-- `is_builtin=true`, `project_id=null` (global, visible from every project —
-- mirrors built-in workflows). `config_json.sources` exactly reproduces
-- CP-44's `defaultContextSourceIDs` (feature.history, chat.summary,
-- source.excerpt — see context_sources_builtin.go) so binding a step to
-- this instance is behavior-preserving with the pre-CP-45 default set.
-- `mcp.driver` stays opt-in only (CP-44 P-10/Task-194), so it is
-- deliberately not part of this default instance; a user creates a separate
-- instance (e.g. `sources: ["mcp.driver"]`) to opt in.
--
-- Uses a fixed, well-known id (rather than gen_random_uuid()) so Go-side
-- built-in mirror-sync code (Task-205's built-in flow binding seed) can
-- reference this instance deterministically without a lookup round-trip.
insert into artifact_instances (
  id,
  project_id,
  artifact_type_id,
  name,
  description,
  config_json,
  is_builtin,
  status
) values (
  '00000000-0000-0000-0000-000000000001',
  null,
  'context_artifact.v1',
  'Default Context (All Sources)',
  'Built-in context artifact using the full CP-44 default source set (feature history, chat summary, source excerpts).',
  '{"sources": ["feature.history", "chat.summary", "source.excerpt"]}'::jsonb,
  true,
  'active'
)
on conflict (id) do update
  set name = excluded.name,
      description = excluded.description,
      config_json = excluded.config_json,
      is_builtin = true,
      updated_at = now();
