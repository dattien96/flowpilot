create policy "authenticated write global workflows"
  on workflows for all to authenticated using (
    project_id is null
  ) with check (
    project_id is null
  );

create policy "authenticated write global workflow_steps"
  on workflow_steps for all to authenticated using (
    exists (
      select 1
      from workflows w
      where w.id = workflow_steps.workflow_id
        and w.project_id is null
    )
  ) with check (
    exists (
      select 1
      from workflows w
      where w.id = workflow_steps.workflow_id
        and w.project_id is null
    )
  );
