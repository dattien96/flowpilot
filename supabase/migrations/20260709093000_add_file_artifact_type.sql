-- CP-45 Task-202: second built-in artifact type, `file_artifact.v1`, proving
-- the framework is not hardcoded to context (SD-23 D-8). Config is an
-- explicit bounded `paths: []` list; the runner resolver
-- (fileArtifactResolver, artifact_type_registry.go) reuses
-- readSourceExcerpts' workspace-safe reader, so path-escape protection is
-- identical to source.excerpt's, not re-implemented.
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
  'file_artifact.v1',
  1,
  'file',
  '',
  '{"promptSection": "file"}'::jsonb,
  '{"type": "object", "required": ["paths"], "properties": {"paths": {"type": "array", "items": {"type": "string"}, "description": "Workspace-relative or absolute paths read live at step runtime (SD-23 D-8), bounded and workspace-safe."}}}'::jsonb,
  'file_excerpt_list',
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
