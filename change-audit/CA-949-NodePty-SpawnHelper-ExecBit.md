# CA-949 — BUG-385: restore +x on node-pty spawn-helper in ensure-deps

## Summary

`term:spawn` died with `posix_spawnp failed` because npm extraction left
`node_modules/node-pty/prebuilds/darwin-*/spawn-helper` without the exec bit.
`ensure-deps.js` now checks/restores `0755` on every run — terminal
self-heals for all devs, no manual chmod.

## Files

- `apps/desktop-flowpilot/scripts/ensure-deps.js` — spawn-helper exec-bit guard

## Verified

- `chmod -x` → `node scripts/ensure-deps.js` → `+x` restored + logged
- `pty.spawn("/bin/zsh")` smoke: output + clean exit

## Provider parity

Provider-agnostic: dev-tooling script only.
