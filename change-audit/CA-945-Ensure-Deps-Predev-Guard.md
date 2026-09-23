# CA-945 — ensure-deps pre-script guard (desktop dev UX)

## Summary

`@xterm/xterm` + `@xterm/addon-fit` were declared in package.json/lockfile but
`node_modules/@xterm` was absent → vite `Failed to resolve import` on
TerminalPanel.tsx. Devs had to know to run `npm install` manually.

Fix: `scripts/ensure-deps.js` runs `npm install` when node_modules is missing
or package.json/package-lock.json is newer than the install stamp
(`node_modules/.package-lock.json`). Wired as `predev`, `prebuild`,
`pretypecheck`, `pretest:phase1` so every entry point self-heals.

## Verified

- Fresh tree: script exits silently (skip).
- `touch package-lock.json` → triggers `npm install` → "up to date".
- `npm run typecheck` → clean (TerminalPanel xterm errors gone).
