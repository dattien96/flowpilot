begin;

alter table if exists step_definitions
  alter column model set default 'gpt-5.4';

update step_definitions
set
  model = 'gpt-5.4'
where model is null
   or trim(model) = ''
   or model = 'gpt-5.5';

commit;
