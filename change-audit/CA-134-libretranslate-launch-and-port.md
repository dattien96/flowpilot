# CA-134: LibreTranslate Launch Detection And Port Routing

## Scope

- LibreTranslate detection and process launch in `scripts/supervisor.js`.
- Default runner translate base URL in `translate_config.go`.
- Desktop settings copy for the local LibreTranslate endpoint and startup command.

## Completed

- Made the supervisor detect LibreTranslate through the resolved binary, common user-site install paths, and the existing `pip show libretranslate` fallback.
- Added env-driven LibreTranslate ports so the local stacks no longer collide on `5000`; defaults now use `5001` for production and `5002` for dev via the tracked env examples.
- Made the runner's default translation base URL derive from `FLOWPILOT_LIBRETRANSLATE_URL` or `FLOWPILOT_LIBRETRANSLATE_PORT`, falling back to `http://127.0.0.1:5001`.
- Updated the desktop settings panel to show the configured LibreTranslate URL and the matching startup command.

## Verification

- `node --check scripts/supervisor.js` passed.
- `npm run typecheck` in `apps/desktop-flowpilot` passed.
- `npm test -- src/lib/env/browser-env.test.ts src/lib/env/app-env.test.ts` in `apps/admin-web` passed.
- `git diff --check` passed.

## Residual Notes

- The installed macOS user-site launcher currently points at the Xcode Python interpreter, so `python -m libretranslate` still does not work in this environment; the supervisor now prefers the discovered user-site script path instead.
- GitNexus MCP tools were unavailable in this session, so impact/detect-changes checks were approximated with local symbol and diff searches.

# ---8<--- flowpilot:change-ledger
feature_key: terminal-session
source_doc_id: Task-160
change_type: feature
summary: Detect LibreTranslate installs reliably and move the local translation port off 5000
# --->8---
