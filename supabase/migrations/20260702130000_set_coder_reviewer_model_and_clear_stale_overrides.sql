-- BUG-162: follow-up to BUG-161. The new flow-agent-delegate-coder/-reviewer
-- step_definitions rows were inserted without an explicit `model`, so they
-- took the column's schema default ('gpt-5.5', a Codex/GPT model) rather than
-- the Claude Haiku the user actually intends for these steps. Set it
-- explicitly.
update public.step_definitions
set model = 'claude-haiku'
where step_type in ('flow-agent-delegate-coder', 'flow-agent-delegate-reviewer');

-- Clear the leftover per-step overrides that BUG-160's now-fixed
-- addWorkflowStep bug stamped onto every step added before that fix landed
-- (model_override='gpt-5.4', reasoning_effort_override='medium', forced
-- unconditionally at creation time). Scoped narrowly on purpose: only the
-- CP-42 generic flow-engine step types, and only rows whose value still
-- exactly matches the old forced default — a workflow_steps row on any other
-- step type, or one whose override was deliberately changed to something
-- else, is left untouched.
update public.workflow_steps
set model_override = null
where step_type in (
  'flow-agent-delegate',
  'flow-agent-delegate-coder',
  'flow-agent-delegate-reviewer',
  'flow-hub-inline',
  'flow-context-produce',
  'flow-context-render',
  'flow-command-validate',
  'flow-validation-summarize',
  'flow-artifact-audit-draft',
  'flow-control',
  'flow-user-confirm'
)
and model_override = 'gpt-5.4';

update public.workflow_steps
set reasoning_effort_override = null
where step_type in (
  'flow-agent-delegate',
  'flow-agent-delegate-coder',
  'flow-agent-delegate-reviewer',
  'flow-hub-inline',
  'flow-context-produce',
  'flow-context-render',
  'flow-command-validate',
  'flow-validation-summarize',
  'flow-artifact-audit-draft',
  'flow-control',
  'flow-user-confirm'
)
and reasoning_effort_override = 'medium';
