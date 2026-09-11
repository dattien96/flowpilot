# CA-826 — hide composer [stop] when the loop is done

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-371
change_type: bugfix
summary: Composer [stop] drops when flowLoopStatus is done and no child is RUNNING; ask_user/approval/gate also hide it
# --->8---

## Why

Live run-678326 chrome was `task 3/3 · [stop] · done` after synthesis
approved. [stop] is live-work chrome.

## Change

- `turnIsActive`: done + !hasLiveWorkingChild → false
- question/approval/gate → false
- RUNNING child on a done loop still arms [stop]

## Tests

New `TestBUG371_*` (Claude/Codex/Grok). Old
`TestFlowDone_NotSettledWhileChildRunning` green.

## Providers

TUI chrome, matrixed 3 providers.

## Will not undo

run-189839 settle. Live-child [stop].
