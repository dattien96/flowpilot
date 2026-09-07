# CA-750 — BUG-359: expanded prompt stays open during drag-select copy

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-359
change_type: bugfix
summary: prompt expand toggle deferred from press-down to same-cell release so drag-select copies long prompts without collapsing; 5 collapse tests converted to real-click pairs with approval
# --->8---

## Problem (operator 2026-09-06)

Long prompts clamp at 4 lines with expand-on-click. Expanding then drag-selecting to copy collapsed the box instantly: the toggle dispatched on mouse PRESS over any box row, and every drag starts with such a press. Wheel-only terminals (no motion events) were doubly broken — release-select copied the collapsed text.

## Change (TUI-only)

- `tui/app/model.go`: `mouseDrag.expandKey` (press-armed expand key, empty otherwise).
- `tui/app/mouse.go`: `armedExpandedPromptKey` (press target is an already-expanded box); press arms drag instead of dispatching (collapsed boxes keep immediate expand; Windows zero-action keeps immediate toggle); release toggles only on same-cell release over the same box, else existing drag-copy path.
- Tests: 3 new (`bug359_expanded_prompt_select_test.go`); `clickLeftFull` helper (press+release); 5 pre-existing collapse sites converted press-only → real-click pair with explicit operator approval 2026-09-06 (they pinned the old press-toggle semantics; assertions untouched): `TestUserPrompt_ExpandCollapseClick`, `TestRegression_YouBoxTrueColorViewAllLines`, `TestRegression_YouBoxUserPastedPromptAllLines`, `TestYouBoxClamp_BareCRPromptCollapsedExpanded`, `TestYouBoxClamp_ChangeContractCollapsedExpanded`.
- No rendering changes: the 5 failures were all interaction-semantics, proven by identical fail set with/without the new test file and zero baseline failures (stash-proven).
