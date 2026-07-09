-- CP-45 Task-197: system-owned artifact type catalog.
--
-- `artifact_types` is the contract layer of the generic artifact framework
-- (SD-23 D-1/D-2): every row is a built-in, versioned type id (e.g.
-- "context_artifact.v1"). Users never create rows here — only
-- `artifact_instances` (Task-198) are user-authored. RLS below grants
-- `authenticated` read-only access; only the service role (migrations /
-- mirror-sync) can write, enforcing "code hardcodes the type layer" (SD-23
-- D-2) at the database boundary, not just in the UI.
--
-- This table is distinct from the pre-existing `artifact_definitions`
-- (20260521091000_add_artifact_definition_catalog.sql), which is a
-- path-template registry for generated documents (PRD.md, TechSpec.md, ...)
-- produced by workflow runs — an unrelated, older concept. CP-45's
-- `artifact_types`/`artifact_instances` model typed grounding data (context
-- packages, file references) flowing between flow steps.
create table if not exists artifact_types (
  id text primary key,
  version int not null default 1,
  category text not null,
  producer_behavior text not null default '',
  consumer_hints jsonb not null default '{}'::jsonb,
  config_schema jsonb not null default '{}'::jsonb,
  render_template text not null default '',
  system_owned boolean not null default true,
  status text not null default 'active' check (status in ('active', 'deprecated')),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

alter table artifact_types enable row level security;

create policy "authenticated read artifact_types"
  on artifact_types for select to authenticated using (true);

-- Intentionally no insert/update/delete policy for `authenticated`: only the
-- service role (used by migrations and any future mirror-sync) may write
-- type rows, matching SD-23 D-2 ("code hardcodes ArtifactType" — users are
-- never able to define a custom type in v1).

insert into artifact_types (
  id,
  version,
  category,
  producer_behavior,
  consumer_hints,
  config_schema,
  render_template,
  system_owned,
  status
) values (
  'context_artifact.v1',
  1,
  'context',
  'context.produce',
  '{"promptSection": "context", "noVectorRetrieval": true}'::jsonb,
  '{"type": "object", "required": ["sources"], "properties": {"sources": {"type": "array", "items": {"type": "string"}, "description": "Enabled ContextSourceRegistry source ids (e.g. feature.history, chat.summary, source.excerpt, mcp.driver)."}}}'::jsonb,
  'context_package_sections',
  true,
  'active'
)
on conflict (id) do update
  set version = excluded.version,
      category = excluded.category,
      producer_behavior = excluded.producer_behavior,
      consumer_hints = excluded.consumer_hints,
      config_schema = excluded.config_schema,
      render_template = excluded.render_template,
      system_owned = excluded.system_owned,
      status = excluded.status,
      updated_at = now();
