alter table if exists step_definitions
  add column if not exists mcp_access_mode text not null default 'read_only';

update step_definitions
set mcp_access_mode = 'read_only'
where mcp_access_mode is null
   or mcp_access_mode not in ('read_only', 'read_write');

alter table if exists step_definitions
  drop constraint if exists step_definitions_mcp_access_mode_check;

alter table if exists step_definitions
  add constraint step_definitions_mcp_access_mode_check
    check (mcp_access_mode in ('read_only', 'read_write'));
