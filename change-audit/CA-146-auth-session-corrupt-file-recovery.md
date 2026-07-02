# CA-146: Auth Session Corrupt File Recovery

Guard `loadPersistedAuthSession` against empty/truncated JSON files and make `savePersistedAuthSession` atomic to prevent the corruption in the first place.

## Changes

- `apps/desktop-flowpilot/electron/main.ts`:
  - Added `rename` to `node:fs/promises` import.
  - `loadPersistedAuthSession`: catches `SyntaxError` from `JSON.parse`, deletes the corrupt file, and returns `null` (treats as no session).
  - `savePersistedAuthSession`: writes to a `.tmp` file then atomically renames to the final path so a mid-write kill can never leave a zero-byte file.

# ---8<--- flowpilot:change-ledger
feature_key: supabase-config
source_doc_id: BUG-151
change_type: bugfix
summary: handle corrupt auth-session JSON file gracefully and make save atomic
# --->8---
