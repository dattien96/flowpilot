-- Migration: Repair legacy flow node relations after moving node definitions.
--
-- Some databases may already have applied the first 20260703160000 migration
-- draft, which copied node fields into shared step_definitions rows such as
-- `flow-agent-delegate-reviewer`. That is still wrong because multiple graph
-- nodes can share those rows. Create one node-specific step_definition per
-- legacy workflow_steps node and repoint workflow_steps.step_type to it.

do $$
begin
  if exists (
    select 1
    from information_schema.columns
    where table_schema = 'public'
      and table_name = 'workflow_steps'
      and column_name = 'node_id'
  ) then
    execute $insert_node_definitions$
      with legacy as (
        select
          ws.id as workflow_step_id,
          trim(both '_' from regexp_replace(
            lower(
              coalesce(nullif(w.pack_id, ''), 'workflow') ||
              '__' ||
              coalesce(nullif(w.pack_flow_id, ''), w.id::text) ||
              '__' ||
              coalesce(nullif(ws.node_id, ''), ws.step_type)
            ),
            '[^a-z0-9]+',
            '_',
            'g'
          )) as new_step_type,
          coalesce(nullif(w.name, ''), 'Flow') || ': ' ||
            initcap(replace(coalesce(nullif(ws.node_id, ''), ws.step_type), '_', ' ')) as new_name,
          ws.node_id,
          ws.behavior_id,
          ws.agent_ref,
          ws.depends_on_json,
          ws.join_mode,
          ws.cohort,
          ws.prompt_template_ref,
          ws.context_ref,
          ws.inputs_json,
          ws.outputs_json,
          sd.description,
          sd.required_mcps,
          sd.required_skills,
          sd.agent_type,
          sd.prompt_base,
          sd.model,
          sd.reasoning_effort,
          sd.yolo_mode,
          sd.mcp_access_mode,
          sd.team_role,
          sd.subagent
        from public.workflow_steps ws
        join public.workflows w on w.id = ws.workflow_id
        join public.step_definitions sd on sd.step_type = ws.step_type
        where ws.node_id is not null
           or ws.behavior_id is not null
           or ws.agent_ref is not null
      )
      insert into public.step_definitions (
        step_type,
        name,
        description,
        required_mcps,
        required_skills,
        agent_type,
        prompt_base,
        model,
        reasoning_effort,
        yolo_mode,
        mcp_access_mode,
        team_role,
        subagent,
        node_id,
        behavior_id,
        agent_ref,
        depends_on_json,
        join_mode,
        cohort,
        prompt_template_ref,
        context_ref,
        inputs_json,
        outputs_json
      )
      select
        new_step_type,
        new_name,
        coalesce(nullif(description, ''), 'Flow node definition migrated from workflow_steps.'),
        coalesce(required_mcps, '[]'::jsonb),
        coalesce(required_skills, '[]'::jsonb),
        coalesce(nullif(agent_type, ''), 'standard'),
        prompt_base,
        model,
        reasoning_effort,
        coalesce(yolo_mode, false),
        coalesce(nullif(mcp_access_mode, ''), 'read_only'),
        team_role,
        subagent,
        node_id,
        behavior_id,
        agent_ref,
        coalesce(depends_on_json, '[]'::jsonb),
        join_mode,
        cohort,
        prompt_template_ref,
        context_ref,
        coalesce(inputs_json, '{}'::jsonb),
        coalesce(outputs_json, '{}'::jsonb)
      from legacy
      on conflict (step_type) do update
      set
        name = excluded.name,
        description = excluded.description,
        required_mcps = excluded.required_mcps,
        required_skills = excluded.required_skills,
        agent_type = excluded.agent_type,
        prompt_base = excluded.prompt_base,
        model = excluded.model,
        reasoning_effort = excluded.reasoning_effort,
        yolo_mode = excluded.yolo_mode,
        mcp_access_mode = excluded.mcp_access_mode,
        team_role = excluded.team_role,
        subagent = excluded.subagent,
        node_id = excluded.node_id,
        behavior_id = excluded.behavior_id,
        agent_ref = excluded.agent_ref,
        depends_on_json = excluded.depends_on_json,
        join_mode = excluded.join_mode,
        cohort = excluded.cohort,
        prompt_template_ref = excluded.prompt_template_ref,
        context_ref = excluded.context_ref,
        inputs_json = excluded.inputs_json,
        outputs_json = excluded.outputs_json;
    $insert_node_definitions$;

    execute $repoint_workflow_steps$
      with legacy as (
        select
          ws.id as workflow_step_id,
          trim(both '_' from regexp_replace(
            lower(
              coalesce(nullif(w.pack_id, ''), 'workflow') ||
              '__' ||
              coalesce(nullif(w.pack_flow_id, ''), w.id::text) ||
              '__' ||
              coalesce(nullif(ws.node_id, ''), ws.step_type)
            ),
            '[^a-z0-9]+',
            '_',
            'g'
          )) as new_step_type
        from public.workflow_steps ws
        join public.workflows w on w.id = ws.workflow_id
        where ws.node_id is not null
           or ws.behavior_id is not null
           or ws.agent_ref is not null
      )
      update public.workflow_steps ws
      set step_type = legacy.new_step_type
      from legacy
      where ws.id = legacy.workflow_step_id
        and ws.step_type <> legacy.new_step_type
    $repoint_workflow_steps$;
  end if;
end $$;
