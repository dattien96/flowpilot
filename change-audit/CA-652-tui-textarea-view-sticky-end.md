# CA-652: TUI textarea View-when-sticky-end and mirror fidelity (Task-308 Phase 2)

## What

Phase 2 of Task-308 makes the composer render via `textarea.View()` only on
the sticky-end draft path, keeping the legacy custom renderer for all other
cases (empty idle, mid-string caret, skill highlight, attach chip, burst,
live turn, blocked/attention). This keeps the CA-633 idle-pin/caching
contract intact.

- `chat_textarea.go`: `newChatTextArea` `Prompt=""` (was `┃ `) to avoid
  double prompt with `frameInput`; `useTextareaView()` real predicate:
  `mirrorReady && authNone && !child && !modal && !burst && cursor<0 &&
  draft!="" && no skills/attach/blocked/gate/question/approval/attention/live`;
  `normalizeComposerForMirror` + `expandMirrorTabs` keep the mirror
  byte-faithful to the bubbles sanitizer (`\r`/`\n`->`\n`, `\t`->4 spaces);
  `syncTextareaValue` guarded by `Value()==norm` to avoid churn.
- `app.go:renderInputLine`: sticky-end draft `syncTextareaValue()` +
  `SetWidth(innerW)` + `SetHeight(LineCount)` (no 1..8 clamp per CA-560) +
  Focus/Blur on `cursorOn` so idle pin still recomposes; View lines prepended
  with attach chip only on fallback path (View now excluded when attach
  present); `frameInput` still wraps the result.
- Tests: `task308_textarea_test.go` 13-18 rewritten to assert the predicate
  honestly (sticky-end true, empty/mid/skill/attach/burst false), placeholder
  via `textarea.Placeholder` not render, Prompt empty, attach fallback.

## Why

Round 3 review found 916830f was a gated scaffold (`return false`) with dead
View code and tests that could not catch a View bug. Re-enabling View naively
broke CA-633 idle (`composeBuilds` stayed 1) because `textarea.View()` does
not read `m.cursorOn` and empty placeholder made pin look like a cache hit.

## Tests

- `task308_textarea_test.go`: View_WhenStickyEnd (predicate true, Prompt empty,
  View contains draft), Empty (predicate false, placeholder via field),
  FallsBack* (mid/skill/attach/burst/auth), MirrorNormalizesTabAndCRLF,
  RejectRevertSyncsMirror, CollapseBurstSyncsMirror.
- `go vet ./internal/tui/app` clean; `go test ./internal/tui/...` 6/6 pass
  (including `TestCA633_*`).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-308
change_type: refactor
summary: enable textarea View on sticky-end draft, keep legacy fallback for idle/burst/skill/attach/live/blocked, fix mirror fidelity
# --->8---
