# SD-24 Capability Evidence — Gemini CLI

- Provider: Google Gemini CLI (one-shot / headless)
- Date: 2026-07-17
- Status: **DEFERRED for live multi-turn probe; enable-blocked by default**

## Scoped decision (CP-51 Task-257)

Until a full live acceptance/reconcile/attach probe is recorded, Gemini remains:

- **V2 automated dispatch disabled by default** (`providerV2Enabled("gemini") == false`)
- Opt-in experimental only via `FLOWPILOT_DISPATCH_V2_PROVIDERS=gemini` (still **no** `TurnBridge.Accepted` call site)

## Matrix cells (current)

| Capability | Outcome |
|------------|---------|
| Session | process start (not a receipt) |
| Prompt send | one-shot process |
| Acceptance receipt | **none before first output** / deferred → no `Accepted` seam |
| Query after kill | **none** documented → `uncertain` if needed |
| Attach | **none** |

## Explicit negative rule

Process start and first stdout line are **not** receipts. Do not call `TurnBridge.Accepted` without Task-257 evidence naming a stable ReceiptID distinct from completion.

## Enable path

Same as Claude: live probe → evidence file → product approve → default enable or permanent three-outcome allow-list.
