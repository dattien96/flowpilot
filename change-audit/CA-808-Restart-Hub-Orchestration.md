# CA-808 — restart must not kill hub orchestration

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: Restore autoOrchestrate on flow-topology resume paths and hub dispatch; arm the stall watchdog when a hub defer has no driver
# --->8---

## Why

Live run-220036: OK drove coder DONE, validate passed (exit 0), synthesis
RUNNING — then silence. Diag: `hub_notify_reinvoke_deferred`, nothing
after. `normalizeResumedFlowRun` strips autoOrchestrate on every restart of
an incomplete flow while the topology survives, so every hub reinvoke
defers+skips (no re-arm, no stall timer): synthesis can never start.
Deliverable survived anyway: snake tests green on disk.

## Change

- `dispatchHubNotifyNode`, pause-gate OK, `maybeAdvance…`, hub_stalled
  Retry self-heal: flow topology restores autoOrchestrate (Failed and
  sealed loops still refuse).
- Both hub defer paths arm the stall watchdog when nothing re-armed on a
  running loop: silence becomes an actionable card.
- Restored two `return`s dropped by the watchdog edit (fall-through would
  double-unlock and mutate without the lock).

## Tests

New ca808_restart_hub_orchestration_test.go (3 pass). Full CA-801→806
chain green. Stop contract untouched: a sealed run still needs a fresh run.

## Providers

Agnostic Case 1.

## Will not undo

BUG-248 Stop-wins, BUG-288 terminal, V10R resume normalize (only heals
downstream of an explicit user resume).
