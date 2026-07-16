# SD-24 Capability Evidence — Gemini CLI (deferred)

- Provider: Google Gemini CLI (one-shot)
- Status: **DEFERRED** (CP-51 `CE-CL/GEM`, Task-257)
- Date: 2026-07-16

## Scope

Gemini is **V2-disabled** until a live Task-257 probe records acceptance receipt, reconcile, and attach outcomes. No production adapter may call `TurnBridge.Accepted` for Gemini under this status.

## Interim rule

- `providerV2Enabled("gemini") == false`
- Automated V2 dispatch rejects Gemini with `provider_v2_disabled`
- Process start / first output is **not** a receipt (SD-24 §6.3)

## Follow-up

Run the Task-257 live probe harness and replace this deferred note with the recorded matrix cells before enabling Gemini on V2.
