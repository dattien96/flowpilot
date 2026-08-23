# CA-608: normalize CRLF and bare CR newlines in wrapText for prompt box clamp and rendering (TUI)

## What

Operator reported: "TUI ui bị lỗi khi show pmompt box. rule là show max 4 line. nếu dài hơn thì show ... và cho phép click để expand." — The screenshot showed a 5-line Change Contract prompt (`[Change Contract]\rfeature: calc-core\rintent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a\rfiles: calc.go, calc_test.go\rsymbols: Subtract`) where bare carriage return (`\r`) or CRLF line separators caused `strings.Split(s, "\n")` in `wrapText` to treat the prompt as a single paragraph. As a result, the 5 lines were not recognized as 5 lines, the 4-line clamp did not trigger, and the embedded `\r` caused terminal cursor resets that visually overwrote preceding lines onto the same terminal row with cursor artifact blocks.

## Why

- Prompts originating from pasted clipboards, NDJSON turns (e.g. `run-204658`), or Windows tools may use `\r\n` (CRLF) or bare `\r` (CR) line separators.
- `wrapText` in `apps/local-runner/internal/tui/app/helpers.go` previously only split by `\n`, leaving `\r` embedded in the wrapped paragraphs.
- Normalizing `\r\n` -> `\n` and `\r` -> `\n` in `wrapText` ensures all newlines are cleanly recognized, properly wrapped, clamped at 4 lines with `....` tail when collapsed, expanded on click, and rendered without carriage return artifacts.

## Fix

- `apps/local-runner/internal/tui/app/helpers.go`:
  - In `wrapText(s string, width int) []string`, normalize `s = strings.ReplaceAll(s, "\r\n", "\n")` and `s = strings.ReplaceAll(s, "\r", "\n")` before paragraph splitting.

## Tests

- Added `apps/local-runner/internal/tui/app/you_box_crlf_clamp_test.go`:
  - `TestWrapText_NormalizesNewlines`: verifies LF, CRLF, bare CR, and mixed line breaks produce identical split slices.
  - `TestYouBoxClamp_BareCRPromptCollapsedExpanded`: verifies exact bare CR prompt from operator repro clamps to 4 lines with `....` tail, hides 5th line (`symbols: Subtract`), expands to full 5 lines on click, and collapses on second click across Claude, Codex, and Grok at width 197 and 80.
  - `TestYouBoxClamp_CRLFPromptCollapsedExpanded`: verifies CRLF prompt clamps to 4 lines with `....` tail and expands cleanly across Claude, Codex, and Grok.
  - `TestYouBoxClamp_BareCRCopyChip`: verifies `[copy]` chip works correctly on bare CR prompt without triggering expansion.
- All existing prompt box and regression tests pass (`TestWrapText`, `TestYouBox*`, `TestUserPrompt*`, `TestRegression_YouBox*`).

## Provider parity

Provider-agnostic text wrapping and prompt box rendering in TUI. Verified on Claude, Codex, and Grok via parameterized test cases.

## Not undone

- Preserves CA-607 4-line clamp and click-to-expand behavior.
- Preserves CA-604 `[copy]` chip on own row.
- Preserves CA-602 plain full-width You box frame.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: normalize CRLF and bare CR newlines in wrapText so user prompt boxes clamp to 4 lines with '....' ellipsis and expand on click without terminal carriage-return overwrite artifacts
# --->8---
