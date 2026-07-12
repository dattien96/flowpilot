-- Task-225: optional per-instance output structure for file_artifact.v1.
-- paths remains required; structure is optional markdown_sections + sections[].
-- No separate format/preset field. Paths-only instances stay valid.

update artifact_types
set
  config_schema = '{
    "type": "object",
    "required": ["paths"],
    "properties": {
      "paths": {
        "type": "array",
        "items": { "type": "string" },
        "description": "Workspace-relative or absolute paths (SD-23 D-8), bounded and workspace-safe."
      },
      "structure": {
        "type": "object",
        "required": ["kind", "sections"],
        "properties": {
          "kind": {
            "type": "string",
            "const": "markdown_sections",
            "description": "Only markdown_sections is supported in v1 (Task-225)."
          },
          "sections": {
            "type": "array",
            "items": { "type": "string" },
            "minItems": 1,
            "description": "Required ATX section titles (flex match: level, case, whitespace). Missing structure means existence-only gate."
          }
        },
        "additionalProperties": false
      }
    },
    "additionalProperties": false
  }'::jsonb,
  updated_at = now()
where id = 'file_artifact.v1';

-- Drop discarded Task-225 draft field if any row still carries format.
update artifact_instances
set
  config_json = config_json - 'format',
  updated_at = now()
where artifact_type_id = 'file_artifact.v1'
  and config_json ? 'format';

-- Best-effort: if a project already has a coder-summary style file instance,
-- attach the coding memo sections so live CP-45 sandbox keeps template + gate.
-- Safe no-op when no such row exists. Does not invent new instances.
update artifact_instances
set
  config_json = coalesce(config_json, '{}'::jsonb) || jsonb_build_object(
    'structure', jsonb_build_object(
      'kind', 'markdown_sections',
      'sections', jsonb_build_array('What', 'Why', 'Baseline')
    )
  ),
  updated_at = now()
where artifact_type_id = 'file_artifact.v1'
  and is_builtin = false
  and config_json ? 'paths'
  and (
    config_json::text ilike '%coder-summary%'
    or config_json::text ilike '%coder_summary%'
  )
  and not (config_json ? 'structure');
