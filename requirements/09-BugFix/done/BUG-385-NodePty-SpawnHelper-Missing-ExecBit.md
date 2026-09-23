# BUG-385: Terminal fails to start — `posix_spawnp failed`

- Document ID: `BUG-385`
- Status: `done`
- Severity: high — embedded terminal (CP-83) unusable on macOS
- Area: `apps/desktop-flowpilot/scripts/ensure-deps.js` (auto-heal), `node-pty` prebuilds

## Symptom

Opening the terminal panel shows
`Terminal failed to start: Error invoking remote method 'term:spawn': Error: posix_spawnp failed.`

## Root cause

node-pty on darwin spawns shells through
`node_modules/node-pty/prebuilds/darwin-*/spawn-helper`. After `npm install`
extracted the prebuild, `spawn-helper` had mode `-rw-r--r--` — no exec bit —
so `posix_spawnp` on the helper failed for every spawn. Confirmed: `chmod +x`
alone makes `pty.spawn("/bin/zsh")` succeed.

## Fix

`ensure-deps.js` (already wired into `predev`/`prebuild`/`pretypecheck`/
`pretest:phase1`) now asserts the exec bit on every
`node_modules/node-pty/prebuilds/*/spawn-helper` and restores `0755` when
missing — self-healing on each dev command, not only post-install.

## Verified

- Stripped `+x` on `darwin-arm64/spawn-helper` → `node scripts/ensure-deps.js`
  restored `0755` with log line.
- Direct `pty.spawn("/bin/zsh")` smoke test: shell output received, clean exit.
