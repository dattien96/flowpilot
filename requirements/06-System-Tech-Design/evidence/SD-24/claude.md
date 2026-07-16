# SD-24 Capability Evidence — Claude CLI

- Provider: Anthropic Claude Code CLI (`claude`)
- Date: 2026-07-17
- Status: **DEFERRED for live multi-turn probe; enable-blocked by default**

## Scoped decision (CP-51 Task-257)

Until a full live acceptance/reconcile/attach probe is recorded, Claude remains:

- **V2 automated dispatch disabled by default** (`providerV2Enabled("claude") == false`)
- Opt-in experimental only via `FLOWPILOT_DISPATCH_V2_PROVIDERS=claude` (still **no** `TurnBridge.Accepted` call site — stdin write is not a receipt)

## Matrix cells (current)

| Capability | Outcome |
|------------|---------|
| Session/spawn | CLI process spawn (not a receipt) |
| Prompt send | `writeUserTurn` stdin — **not** a receipt |
| Acceptance receipt | **unproven / deferred** → no `Accepted` seam |
| Query after kill | **unproven / deferred** → `uncertain` if ever mid-flight |
| Attach in-flight | **none** documented; respawn = new turn |

## Explicit negative rule

`TurnBridge.Accepted` must not be wired from Claude adapters based on stdin write success or first stream-json line without a dedicated Task-257 live probe that names `file:function` + ReceiptID.

## Enable path

1. Run live Task-257 harness against `claude`.
2. Replace this file with probe transcript + matrix conclusions.
3. Only then set default `providerV2Enabled` or document permanent `unprovable ⇒ uncertain` and allow-list with three-outcome only.
