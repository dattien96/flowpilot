# CA-938 — BUG-376: persist rotated auth tokens after session restore

## Summary

`persistRunnerSession` persisted the request payload after a successful
`setSession` — but Supabase rotates refresh tokens on use, so the stored
refresh token was dead the moment it was written. The next cold start
restored with stale credentials and the desktop fell back to the login
screen — the CP-81 "đang dùng app thì tự nhiên bị out ra" symptom class.

Fix: prefer `toPersistedAuthSession(data.session, clientKey)` (the rotated
tokens) and keep the payload fallback only when the response session lacks
tokens. The `setSession`-failure branch still persists `payload` — best
available credential there.

## Verified

- New additive test in `desktopSupabaseAuthRepository.test.ts`: stubbed
  rotated setSession response → persisted refresh token must be the rotated
  one. Red before fix ("stale-refresh-token"), green after.
- Existing auth test unchanged and green.

## Files

- `src/auth/desktopSupabaseAuthRepository.ts`,
  `tests/phase1/desktopSupabaseAuthRepository.test.ts` (additive),
  `requirements/09-BugFix/todo/BUG-376-…md` (new)

# ---8<--- flowpilot:change-ledger
feature_key: supabase-config
source_doc_id: BUG-376
change_type: bugfix
