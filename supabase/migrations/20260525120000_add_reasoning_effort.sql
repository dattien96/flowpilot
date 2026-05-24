-- Migration: Add reasoning effort fields across configuration and runtime tables
ALTER TABLE projects ADD COLUMN IF NOT EXISTS default_reasoning_effort TEXT;
ALTER TABLE workflows ADD COLUMN IF NOT EXISTS reasoning_effort_override TEXT;
ALTER TABLE workflow_steps ADD COLUMN IF NOT EXISTS reasoning_effort_override TEXT;
ALTER TABLE workflow_runs ADD COLUMN IF NOT EXISTS reasoning_effort TEXT;
ALTER TABLE ai_runs ADD COLUMN IF NOT EXISTS reasoning_effort TEXT;
