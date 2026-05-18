# Logic Sync Report - CP-01 Foundation Setup

Date: 2026-05-18

## Source Checked

- `requirements/07-Coding-Plan/CP-01-Foundation-Setup.md`
- `implementation_plan.md`
- `tdd_signatures.md`

## Requirement Alignment

| Requirement | Status | Evidence |
|---|---|---|
| Vite + React + TypeScript foundation | `[SYNCED]` | `apps/admin-web/package.json`, `apps/admin-web/vite.config.ts`, `apps/admin-web/src/main.tsx` |
| TanStack Router file-based routing | `[SYNCED]` | `apps/admin-web/src/routes/**`, generated `apps/admin-web/src/routeTree.gen.ts`, `apps/admin-web/src/router.tsx` |
| TanStack Query root wiring | `[SYNCED]` | `apps/admin-web/src/app.tsx`, `apps/admin-web/src/lib/query-client.ts` |
| Browser-only Supabase client | `[SYNCED]` | `apps/admin-web/src/data/supabase/client.ts`, `apps/admin-web/src/lib/env/browser-env.ts` |
| Auth guard and protected layout | `[SYNCED]` | `apps/admin-web/src/features/auth/require-auth.ts`, `apps/admin-web/src/routes/_authenticated.tsx`, `apps/admin-web/src/features/auth/require-auth.test.ts` |
| AppShell layout with sidebar, header, content | `[SYNCED]` | `apps/admin-web/src/components/layout/app-shell.tsx` |
| CP-01 route surface | `[SYNCED]` | `apps/admin-web/src/routes/_authenticated/**`, `apps/admin-web/src/routes/route-map.test.ts` |
| Domain constants/contracts remain portable | `[SYNCED]` | Existing `apps/admin-web/src/domain/**` plus `apps/admin-web/src/domain/constant/status.test.ts` |
| Backup of pre-migration app | `[SYNCED]` | `apps/admin-web-nextjs-backup/` |

## Test Alignment

- `[SYNCED]` `src/features/auth/require-auth.test.ts` verifies authenticated session return and guest redirect behavior.
- `[SYNCED]` `src/routes/route-map.test.ts` verifies the required CP-01 route registrations.
- `[SYNCED]` Existing domain and env tests still pass under the Vite test harness.
- `[SYNCED]` `src/features/auth/auth-provider.test.tsx` verifies session hydration and demo fallback behavior.
- `[SYNCED]` `src/components/layout/app-shell.test.tsx` verifies shell navigation and child rendering.

## Notes

- Legacy Next.js files still exist in the repo as inactive reference material, but the active Vite entry path no longer imports `next/*`.
- `npm run build` passes.
- `npm run test` passes.
