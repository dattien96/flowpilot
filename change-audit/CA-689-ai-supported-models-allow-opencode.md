# CA-689 — ai_supported_models check constraint allows opencode

## Problem

Desktop "Detect models" for Opencode (CP-57 Task-302, OC-25) failed on insert:

```
new row for relation "ai_supported_models" violates check constraint
"ai_supported_models_provider_key_check"
```

Root cause: the `ai_supported_models.provider_key` column carries a Supabase
CHECK constraint enumerating the original four providers. CP-57 §6 assumed
`provider_key` was opaque free text ("no schema change") — wrong. Live census
before the fix: codex=8, claude=4, gemini=5, grok=4, opencode impossible.

## Fix

Migration `supabase/migrations/20260830003000_ai_supported_models_allow_opencode.sql`:
recreates the check with `('codex','claude','gemini','grok','opencode')`,
guarded so it only drops the old constraint while it still lacks `opencode`
(idempotent). **Requires manual apply** (PostgREST cannot run DDL): Supabase
SQL editor or `supabase db push`. Code paths unchanged — the insert already
sends the right fields (supabaseAdminRepository.createSupportedModel).

## Verification

- After applying: Settings → Detect models inserts the opencode catalog
  (80+ rows with `provider_key='opencode'`); the constraint rejection no
  longer occurs.
- `select provider_key, count(*) from ai_supported_models group by 1;`
  should gain an opencode bucket.

## Follow-up (CA-690 review)

The first draft left `alter table ... add constraint` outside the guarded
DO block, so a re-run failed with "constraint already exists". Rewritten
truly idempotent: the whole drop-if-exists + add fires only while the
constraint definition lacks 'opencode'; re-runs against a migrated DB are
no-ops.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CP-57
change_type: bugfix
summary: Supabase migration widens the ai_supported_models provider_key check to include opencode so detect-models sync succeeds
# --->8---
