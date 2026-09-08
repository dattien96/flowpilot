# CA-779 — vibe_lock bar shows [Lock] not [Retry]

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-60
change_type: bugfix
summary: SS/CP lock park labels the continue target [Lock]; target stays retry so feedback remains continue
# --->8---

## Why

Live run-213333: SS Preview & Lock parked correctly (`vibe_lock`) but chips were Retry/Stop/Revise. Operators could not see a lock/approve action. Same class as plan_approval [Approve] (run-206538). `resumeVibeLock` already treats feedback `continue` as lock.

## Change

- `actionRingItems` / `renderBlockedBar`: `vibe_lock` → `[Lock]`
- `hitBlockedChrome`: `[Lock]` → retry

## Tests

`ca779_vibe_lock_chip_test.go`. Old plan_approval tests untouched.

## Workaround without rebuild

On the current card, **[Retry] already locks** (sends `continue`).
