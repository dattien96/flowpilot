# CA-006 Fix Local Runner API Proxy in Vite Dev Server

## Scope

Fixed a development-only bug where clicking "Install" or "Refresh" in the AI Providers settings page (`/settings/ai-providers`) did not result in any changes or successful operations in the browser. 

This issue was caused by Vite's dev server middleware (`flowPilotApiRuntime` in `vite.config.ts`) only routing `/api/workflow-engine/start-run` requests to the SSR route handlers, leaving all other `/api/...` calls (such as `/api/local-runner/providers/install`) unhandled (returning a 404/index.html SPA fallback). Additionally, we resolved module resolution failures for Next.js imports (`next/server` and `next/navigation`) when Vite evaluated Next.js API routes under SSR.

## Completed

- Updated `apps/admin-web/vite.config.ts` to implement a dynamic, Next.js-style API route resolver.
- Implemented a recursive directory scanner `scanRoutes` to find all `route.ts` or `route.js` files under `src/app/api/`.
- Translated pathnames and dynamic route segments (e.g. `[approvalId]`, `[artifactId]`, `[runId]`) into regular expression patterns for dynamic request matching.
- Wired the middleware to automatically load matched route modules via `server.ssrLoadModule` and delegate requests to the corresponding HTTP method exports (e.g. `GET`, `POST`, `PUT`, `DELETE`, `PATCH`).
- Extracted and forwarded route parameters (e.g. `approvalId`, `artifactId`) wrapped in `context.params` to match Next.js Route Context expectations.
- Created [next-shim.ts](file:///c:/working/flowpilot/apps/admin-web/src/lib/shims/next-shim.ts) to provide mock/shim implementations of Next.js-specific imports (`NextResponse`, `redirect`, `notFound`, etc.).
- Aliased `next/server` and `next/navigation` imports in `vite.config.ts` to resolve to the new compatibility shim.
- Handled thrown redirects (`RedirectError` from the shim) within `vite.config.ts`'s route processor to issue proper 302 HTTP redirects.

## Verification

- Verified the Vite dev server configuration loads successfully and runs tests.
- Executed tests in `apps/admin-web` via `npm run test` (all 129 tests passed successfully).

## Residual Notes

- Same-origin API calls made by the browser in the dev server now correctly execute the local runner and other backend mocks configured in `src/app/api/...`.
- No changes were made to production Next.js API routing.
