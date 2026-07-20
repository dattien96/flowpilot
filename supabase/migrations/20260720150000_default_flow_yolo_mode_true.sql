-- Flow/Workflow launches (single-step and multi-step alike) always run YOLO=true — the
-- gate/approval/reinvoke machinery a workflow's own hub/cohort orchestration depends on is
-- not robust against a paused approval card mid-flow. YOLO=false is only supported in
-- Normal Chat, which has no hub/cohort/gate layer to race against. The desktop Settings UI
-- now shows and submits yolo_mode=true unconditionally for every workflow/step draft, but a
-- built-in workflow's mirror-sync insert (SyncBuiltins) never sets yolo_mode itself (it is a
-- pure per-installation admin setting, intentionally not part of the pack schema) — a fresh
-- mirror row therefore always fell through to this column's own default, which was false.
--
-- This flips that default to true so a newly-mirrored or newly-created workflow/step
-- (including a first-time "Review Loop" mirror) starts YOLO=true without requiring an admin
-- to open Settings first. Existing rows already persisted with yolo_mode=false are NOT
-- updated by this migration — only the column default for future inserts changes.
alter table if exists workflows
  alter column yolo_mode set default true;

alter table if exists step_definitions
  alter column yolo_mode set default true;
