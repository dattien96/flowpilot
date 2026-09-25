# CA-988 — BUG-489: provider switch persists the closed leg durably or marks it degraded

## What changed

`apps/local-runner/internal/runner/chat_switch.go`:

- New `persistSwitchLegCloseLocked(src)`: persists the old leg's
  `LegStateClosed` + `LegClosedReasonProviderSwitch`; retries once on
  failure; on a second failure logs loudly and marks the leg degraded via
  `chatRuns.setDegraded` so the chat timeline shows the uncommitted close.

Replaces the fire-and-forget `_ = s.persistProviderSession(...)` in the
switch finalization path (Phase C).

## Why

Phase A already fail-closed on the intent marker persist, but the
closed-leg write in Phase C was discarded. A failure left disk saying
`active` while memory said `closed` — on restart both the old and new leg
rehydrated as active for one chat, a dual-active split-brain.

## Contract

- A durable intent failure still aborts the switch (502) — unchanged.
- Close-persist failure no longer claims a clean switch: one retry, then
  degraded flag + log. The switch itself stays committed (new leg already
  exists); the uncommitted close is visible instead of silent.
- Seed-failure semantics (SD26-X-7 `switch_seed_failed` record) unchanged.

## Verification

- `bug489_leg_close_persist_test.go`: transient persist failure retries and
  clears; permanent failure marks the leg degraded; healthy path unchanged.
- Runner package suite green.
