-- CP-45 Task-198: artifact instances + step-to-instance bindings.
--
-- `artifact_instances` holds user-authored (and, via `is_builtin=true`,
-- system-seeded) configurations of a built-in `artifact_types` row (SD-23
-- D-1/D-3). `project_id` is nullable so built-in instances can be global
-- (visible from every project), mirroring how `workflows.project_id` is
-- nullable for built-in workflows.
--
-- `step_artifact_bindings` attaches instances to a step's input/output
-- slots. It references `step_definitions(step_type)` — the same
-- step-definition-catalog layer CP-44's `context_sources` column and the
-- legacy `step_input_artifact_definitions`/`step_output_artifact_definitions`
-- tables already bind to (BUG-236: node/step metadata lives on
-- `step_definitions`, never on `workflow_steps`). A step can carry multiple
-- bindings per direction (SD-23 D-1: "danh sách" per input/output).
create table if not exists artifact_instances (
  id uuid primary key default gen_random_uuid(),
  project_id uuid references projects(id) on delete cascade,
  artifact_type_id text not null references artifact_types(id),
  name text not null,
  description text not null default '',
  config_json jsonb not null default '{}'::jsonb,
  is_builtin boolean not null default false,
  status text not null default 'active' check (status in ('active', 'archived')),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists artifact_instances_project_idx on artifact_instances(project_id);
create index if not exists artifact_instances_type_idx on artifact_instances(artifact_type_id);

alter table artifact_instances enable row level security;

create policy "authenticated read artifact_instances"
  on artifact_instances for select to authenticated using (true);

-- Split insert/update/delete (a `with check` alone does not gate DELETE),
-- mirroring 20260701100000_restrict_builtin_workflow_writes_at_rls.sql: a
-- user can freely CRUD their own (is_builtin=false) instances, but can never
-- create, edit, or delete a built-in (is_builtin=true) row directly — those
-- are only written by the service role (migrations / mirror-sync).
create policy "authenticated insert artifact_instances"
  on artifact_instances for insert to authenticated
  with check (is_builtin = false);

create policy "authenticated update artifact_instances"
  on artifact_instances for update to authenticated
  using (is_builtin = false)
  with check (is_builtin = false);

create policy "authenticated delete artifact_instances"
  on artifact_instances for delete to authenticated
  using (is_builtin = false);

create table if not exists step_artifact_bindings (
  id uuid primary key default gen_random_uuid(),
  step_definition_id text not null references step_definitions(step_type) on delete cascade,
  direction text not null check (direction in ('input', 'output')),
  slot_name text not null default '',
  artifact_instance_id uuid not null references artifact_instances(id) on delete cascade,
  required boolean not null default true,
  position int not null default 0,
  created_at timestamptz not null default now(),
  unique (step_definition_id, direction, artifact_instance_id)
);

create index if not exists step_artifact_bindings_step_idx on step_artifact_bindings(step_definition_id);
create index if not exists step_artifact_bindings_instance_idx on step_artifact_bindings(artifact_instance_id);

alter table step_artifact_bindings enable row level security;

-- Matches the existing sibling tables' RLS posture
-- (step_input_artifact_definitions / step_output_artifact_definitions,
-- 20260521091000): open read/write to `authenticated`. Built-in-workflow
-- read-only protection for steps is enforced at the authoring-UI layer
-- (WorkflowsSettings.tsx already gates the whole step editor off
-- `selectedWorkflow.editable === false`), not by inspecting which workflow a
-- shared step_type happens to belong to here.
create policy "authenticated read step_artifact_bindings"
  on step_artifact_bindings for select to authenticated using (true);
create policy "authenticated write step_artifact_bindings"
  on step_artifact_bindings for all to authenticated using (true) with check (true);
