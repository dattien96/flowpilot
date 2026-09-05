# BUG-359: expanding long prompt collapses on drag-select press — uncopyable

## Metadata

- Document ID: `BUG-359`
- Title: `expanded prompt collapses on drag-select press, long prompts uncopyable`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-06`
- Last Updated: `2026-09-06`
- Feature Keys: `cli-tui`
- Parent Documents: `none`
- Child Documents: `none`
- Related Documents: `none`
- Replaces: `none`
- Tags: `cli-tui, mouse, copy, prompt-clamp`

## AI Quick View

### Summary

- Symptom (operator 2026-09-06): long user prompt expands, but starting a drag-select to copy collapses it instantly — mousedown already toggles.
- Root cause: `hitUserPromptChrome` maps EVERY box row to the expand toggle and `handlePlainLeftMouse` dispatches on PRESS-down — the collapse fires before any drag/motion exists (wheel-only terminals doubly broken: no motion events, release-select copied collapsed text).
- Fix: press on an already-expanded box arms drag-select instead of toggling; toggle fires on same-cell release (real click). Collapsed boxes keep immediate whole-box expand.

### Current Ask

- Done (CA-750). 5 pre-existing collapse tests updated press-only → real-click pair with explicit operator approval (they pinned the old press-toggle semantics).

### Key Decisions

- D-1: Scoped to the prompt-expand target only (no global press→release dispatch change — too wide a blast radius for chips/steps/cards).
- D-2: Same-cell release still collapses (zero behavior loss for clickers); press→release on different cells copies and never toggles (both motion and wheel-only modes).
- D-3: Windows zero-action events keep immediate toggle (no release pairing exists there).

### Constraints

- Additive tests for the new behavior; 5 old-test collapse sites updated press→press+release ONLY (approved 2026-09-06), assertions untouched.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/tui/app/mouse.go` (`hitUserPromptChrome`, `handlePlainLeftMouse`, `armedExpandedPromptKey`)
- `apps/local-runner/internal/tui/app/model.go` (`mouseDrag.expandKey`)

## Evidence

- Operator report + screenshot-equivalent repro: expand long prompt → mousedown on text → box collapses before selection.
- 5 old tests failed post-fix, all "second click must collapse" via press-only `clickLeft` (no rendering regression).

## Root Cause

Toggle dispatched on mouse PRESS over any prompt-box row; drag-select also starts with a press on those rows. Mutually exclusive on the same event.

## Fix direction (implemented)

- `mouseDrag.expandKey`: press on expanded box arms drag (no dispatch); release on same box toggles; release elsewhere = drag-select copy path.
- `clickLeftFull` test helper (press+release); 5 collapse sites converted.
- Tests: 3 new (`bug359_expanded_prompt_select_test.go`: press-no-collapse, same-cell-release collapses, drag-copies-without-collapse incl. wheel-only); full `tui/...` green.

## Completion Notes (implemented 2026-09-06, CA-750)

- Full `go test ./internal/tui/...` PASS; `go vet` clean; gofmt clean on new lines (model.go flag is pre-existing churn elsewhere in file).
