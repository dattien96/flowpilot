# CA-199: Set Coder/Reviewer Haiku Default And Clear Stale Overrides

## Summary

Fixed `BUG-162`, a direct follow-up to `BUG-161`: set the new Coder/Reviewer step types' catalog default model to Claude Haiku, and cleared the leftover Codex/medium overrides that `BUG-160`'s now-fixed `addWorkflowStep` bug had stamped onto steps created before that fix landed.

## What Changed

- `supabase/migrations/20260702130000_set_coder_reviewer_model_and_clear_stale_overrides.sql`: sets `step_definitions.model = 'claude-haiku'` for `flow-agent-delegate-coder`/`flow-agent-delegate-reviewer`; clears `workflow_steps.model_override`/`reasoning_effort_override` back to `null` wherever a CP-42 generic flow-engine step's value still exactly matches the old forced-default (`gpt-5.4`/`medium`).

## Verification

- Reviewed for scope safety: both clearing updates require an exact stale-value match plus a step type in the fixed generic list.
- Not verified live (no Supabase available in this environment) — flagged in `BUG-162` (`V-2`).

## Notes

- No application code changed — `BUG-160`'s `LoadRunSteps` fallback chain already reads `step_definitions.model` when no override exists; this just gives it the value the user actually wants.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-162
change_type: bugfix
summary: Set Coder/Reviewer step types' default model to Claude Haiku and clear stale forced overrides left over from the now-fixed addWorkflowStep bug
# --->8---
