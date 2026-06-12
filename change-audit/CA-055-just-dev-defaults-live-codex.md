# CA-055: Just Dev Defaults Live Codex For Runner

## Summary

- Defaulted the runner spawned by `scripts/supervisor.js` to `FLOWPILOT_CODEX_APPSERVER=1` when `just dev` starts it.
- Preserved an explicit caller-provided `FLOWPILOT_CODEX_APPSERVER` value, so the live Codex path can still be disabled intentionally.

## Changed Files

- `scripts/supervisor.js`

## Verification

- `node --check scripts/supervisor.js`
