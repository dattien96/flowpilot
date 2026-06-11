alter table public.step_definitions
  drop column if exists yolo_mode;

alter table public.workflow_steps
  drop column if exists yolo_mode;
