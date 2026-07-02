# CA-203: Reseed Missing Claude Supported Models

## Summary

Fixed `BUG-166`: `claude-haiku` (and defensively `claude-sonnet`/`claude-opus`) was missing from `ai_supported_models` on the user's live deployment despite being in the original seed migration, causing the Step Definitions detail editor's Model dropdown to display the wrong selection for any Claude-configured step type.

## What Changed

- `supabase/migrations/20260702150000_reseed_missing_claude_supported_models.sql`: idempotently re-inserts the three Claude seed rows (`on conflict do nothing`).

## Verification

- Reviewed for safety: purely additive, cannot overwrite an existing/edited row.
- Not verified live (no Supabase access in this environment) — user should re-run the diagnostic query after applying to confirm.

## Notes

- Confirmed via a live query (zero rows for `model_id = 'claude-haiku'`) rather than static code review — the application code path (`mapStepDefinition`/`stepDraft`/the Model select) was already correct; this was pure data drift.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-166
change_type: bugfix
summary: Re-seed missing Claude rows in ai_supported_models so the Step Definitions Model dropdown can correctly display/select claude-haiku/sonnet/opus
# --->8---
