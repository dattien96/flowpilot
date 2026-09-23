# BUG-376: Persisted auth session stores the consumed refresh token — random logout on next cold start

## Metadata

- Document ID: `BUG-376`
- Title: `persistRunnerSession saves request payload, not rotated session — next boot restores with a dead refresh token`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-23`
- Last Updated: `2026-09-23`
- Feature Keys: `supabase-config`
- Parent Documents: CP-81 (lifecycle/auth persistence; operator-reported "đang dùng app thì tự nhiên bị out ra")
- Child Documents: `none`
- Related Documents: CP-81 lifecycle lease work
- Replaces: `none`
- Tags: `desktop, auth, supabase, refresh-token, session`

## AI Quick View

### Summary

- `persistRunnerSession` (restore path + runner-login fallback) called `supabase.auth.setSession({access_token, refresh_token})`; on success Supabase **rotates** the refresh token (single-use semantics).
- The code persisted `...payload` — the request tokens — so the refresh token saved to the Electron store was the one just consumed and invalidated. `data.session`'s rotated tokens were discarded.
- Next cold start: `getSession()` → no live session → `restorePersistedSession` → `setSession` with the dead refresh token → refresh rejected → session degrades; bootstrap resolves `login` → **user is kicked out mid-use after any reload/restart**. Matches the CP-81 operator report.

### Constraints

- safe-fix-contract: additive change inside `persistRunnerSession` only; existing fallback shape (payload when `data.session` lacks tokens) preserved.

## 4. Expected vs Actual

- expected: after a successful restore, the persisted session carries the fresh rotated tokens, so every future boot restores cleanly.
- actual: the persisted session's refresh token is invalidated at save time; the next cold start restores with dead credentials.

## 5. Root Cause

`persistRunnerSession` built the persisted record from the request `payload` instead of `toPersistedAuthSession(data.session, clientKey)`.

## 6. Fix

- `desktopSupabaseAuthRepository.ts`: on `setSession` success, persist `toPersistedAuthSession(data.session, clientKey)`; fall back to `payload` only when the response session lacks tokens.
- The failure branch (`setSession` threw) still persists `payload` — it is the best available credential.

## 7. Verification

- New test in `tests/phase1/desktopSupabaseAuthRepository.test.ts`: stubbed `setSession` returns rotated tokens; asserted `persistedSession.refreshToken === "rotated-refresh-token"`. Red before fix, green after. Existing auth tests unchanged and green.
