# CA-1135 — BUG-590: gate-config request/response field asymmetry + silent normalize

## What changed

`apps/local-runner/internal/runner/engine_gate_config.go`:

- `setGateConfigRequest` now accepts BOTH `gate_mode` (documented/persisted
  spelling) and `gateMode` (the response's own casing). snake_case wins when
  both are present — `resolvedGateMode()`.
- Unrecognized/empty resolved modes are now `400 invalid_gate_mode` instead
  of silently writing "enforce" and returning 200 — the exact silent-drop an
  operator hit live (`{"gateMode":"warn"}` → 200, still enforce).

## Invariant

A gate-mode write either applies exactly what was asked or fails loudly.
Unknown values never rewrite the persisted mode.

## Tests

`bug590_gate_mode_field_test.go` — red-first: camelCase sets warn; snake
sets enforce; bogus → non-200 + persisted mode untouched.
