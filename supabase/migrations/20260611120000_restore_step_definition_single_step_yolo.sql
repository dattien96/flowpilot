alter table public.step_definitions
  add column if not exists yolo_mode boolean not null default false;
