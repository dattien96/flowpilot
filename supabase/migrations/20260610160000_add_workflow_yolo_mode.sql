alter table if exists workflows
  add column if not exists yolo_mode boolean not null default false;
