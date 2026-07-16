# SD-24 Capability Evidence — Claude CLI (deferred)

- Provider: Anthropic Claude Code CLI
- Status: **DEFERRED** (CP-51 `CE-CL/GEM`, Task-257)
- Date: 2026-07-16

## Scope

Claude is **V2-disabled** until a live Task-257 probe records acceptance receipt, reconcile, and attach outcomes. No production adapter may call `TurnBridge.Accepted` for Claude under this status.

## Interim rule

- `providerV2Enabled("claude") == false`
- Automated V2 dispatch rejects Claude with `provider_v2_disabled`
- stdin write success (`writeUserTurn`) is **not** a receipt (SD-24 §6.3)

## Follow-up

Run the Task-257 live probe harness and replace this deferred note with the recorded matrix cells before enabling Claude on V2.
