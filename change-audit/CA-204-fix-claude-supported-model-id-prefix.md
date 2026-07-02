# CA-204: Fix Claude Supported Model Id Prefix

## Summary

Fixed `BUG-167`, correcting `BUG-166`'s original (wrong) diagnosis: a full `ai_supported_models` export showed the Claude rows use bare ids (`haiku`/`sonnet`/`opus`) instead of the `claude-`-prefixed canonical form (`claude-haiku`/etc.) that `step_definitions.model`, `STEP_MODEL_OPTIONS`, and Go's `providerKeyFromModel` prefix-matching all expect. Removed `BUG-166`'s never-applied insert migration (which would have created a duplicate row) and replaced it with an in-place rename.

## What Changed

- Removed `supabase/migrations/20260702150000_reseed_missing_claude_supported_models.sql` (superseded, never applied anywhere).
- Added `supabase/migrations/20260702160000_fix_claude_supported_model_id_prefix.sql`: deletes a legacy bare-id row if a prefixed duplicate already exists, then renames any remaining bare-id row to the prefixed canonical form.
- `requirements/09-BugFix/done/BUG-166-...md`: metadata updated to `status: superseded`, with a note pointing to `BUG-167`; document body left otherwise unedited as an accurate historical record.

## Verification

- Reviewed the new migration for order-independence (safe whether or not the removed BUG-166 migration ever ran).
- Not verified live — no Supabase access in this environment; user should re-query after applying.

## Notes

- This is a more consequential bug than originally scoped: beyond the Step Definitions dropdown display glitch, a bare `model_id` breaks Go's prefix-based provider derivation for any path that trusts the registry id directly.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-167
change_type: bugfix
summary: Rename ai_supported_models' bare Claude model ids (haiku/sonnet/opus) to the claude-prefixed canonical form required by step_definitions.model and providerKeyFromModel's prefix matching
# --->8---
